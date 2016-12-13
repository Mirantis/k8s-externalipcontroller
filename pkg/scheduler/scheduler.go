// Copyright 2016 Mirantis
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scheduler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"

	"github.com/golang/glog"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	apierrors "k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/fields"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

const (
	AutoExternalAnnotationKey   = "external-ip"
	AutoExternalAnnotationValue = "auto"
)

func NewIPClaimScheduler(config *rest.Config, mask string) (*ipClaimScheduler, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	ext, err := extensions.WrapClientsetWithExtensions(clientset, config)
	if err != nil {
		return nil, err
	}

	serviceSource := &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			return clientset.Core().Services(api.NamespaceAll).List(options)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			return clientset.Core().Services(api.NamespaceAll).Watch(options)
		},
	}

	claimSource := cache.NewListWatchFromClient(ext.Client, "ipclaims", api.NamespaceAll, fields.Everything())
	return &ipClaimScheduler{
		Config:              config,
		Clientset:           clientset,
		ExtensionsClientset: ext,
		DefaultMask:         mask,

		monitorPeriod: 4 * time.Second,
		serviceSource: serviceSource,
		claimSource:   claimSource,

		observedGeneration: make(map[string]int64),
		liveIpNodes:        make(map[string]struct{}),

		queue: workqueue.NewQueue(),
	}, nil
}

type ipClaimScheduler struct {
	Config              *rest.Config
	Clientset           kubernetes.Interface
	ExtensionsClientset extensions.ExtensionsClientset
	DefaultMask         string

	serviceSource cache.ListerWatcher
	claimSource   cache.ListerWatcher

	monitorPeriod      time.Duration
	observedGeneration map[string]int64
	liveSync           sync.Mutex
	liveIpNodes        map[string]struct{}

	claimStore   cache.Store
	serviceStore cache.Store

	queue workqueue.QueueType
}

func (s *ipClaimScheduler) Run(stop chan struct{}) {
	glog.V(3).Infof("Starting monitor goroutine.")
	go s.monitorIPNodes(stop, time.Tick(s.monitorPeriod))
	// lets give controllers some time to register themself after scheduler restart
	// TODO(dshulyak) consider to run monitor both for leaders/non-leaders
	time.Sleep(s.monitorPeriod)
	glog.V(3).Infof("Starting all other worker goroutines.")
	go s.worker()
	go s.serviceWatcher(stop)
	go s.claimWatcher(stop)
	<-stop
	s.queue.Close()
}

// serviceWatcher creates/delets IPClaim based on requirements from
// service
func (s *ipClaimScheduler) serviceWatcher(stop chan struct{}) {
	store, controller := cache.NewInformer(
		s.serviceSource,
		&v1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				svc := obj.(*v1.Service)
				s.processExternalIPs(svc)
			},
			UpdateFunc: func(old, cur interface{}) {
				curSvc := cur.(*v1.Service)
				s.processExternalIPs(curSvc)

				oldSvc := old.(*v1.Service)
				s.processOldService(oldSvc)
			},
			DeleteFunc: func(obj interface{}) {
				svc := obj.(*v1.Service)
				s.processOldService(svc)
			},
		},
	)
	s.serviceStore = store
	controller.Run(stop)
}

//we must take into account that missing of data and double processing of
//service objects might occur (due the way the object cache works); thus
//auto allocation must be done only in case a service is properly annotated
//and there is no already auto allocated IP for it
func (s *ipClaimScheduler) processExternalIPs(svc *v1.Service) {
	foundAuto := false

	pools, err := s.ExtensionsClientset.IPClaimPools().List(api.ListOptions{})
	if err != nil {
		glog.Errorf("Error while retrieving list of IP pools. Details: %v", err)
	}

	for _, ip := range svc.Spec.ExternalIPs {
		if p := poolAllocating(ip, pools); p != nil {
			foundAuto = true
			continue
		}
		err = tryCreateIPClaim(s.ExtensionsClientset, makeIPClaim(ip, s.DefaultMask))
		if err != nil {
			glog.Errorf("Unable to create ip claim %v", err)
		}
	}

	if annotated := checkAnnotation(svc); annotated && !foundAuto {
		s.autoAllocateExternalIP(svc, pools)
	}
}

func poolAllocating(ip string, poolList *extensions.IpClaimPoolList) *extensions.IpClaimPool {
	for _, pool := range poolList.Items {
		if _, exists := pool.Spec.Allocated[ip]; exists {
			return &pool
		}
	}
	return nil
}

func (s *ipClaimScheduler) autoAllocateExternalIP(svc *v1.Service, poolList *extensions.IpClaimPoolList) {
	glog.V(5).Infof("Try to auto allocate external IP for service %v", svc.ObjectMeta.Name)

	var freeIP string
	var pool extensions.IpClaimPool

	for _, p := range poolList.Items {
		ip, err := p.AvailableIP()
		if err != nil {
			glog.Errorf(
				"Fail to retrieve free IP from the pool %v; skipping to a next one. Details: %v",
				p.Metadata.Name, err)
			continue
		}
		freeIP = ip
		pool = p
		break
	}

	if len(freeIP) == 0 {
		glog.Errorf(
			"Fail to provide external IP for service %v. All pools are exhausted.",
			svc.ObjectMeta.Name)
		return
	}

	mask := strings.Split(pool.Spec.CIDR, "/")[1]

	ipclaim := makeIPClaim(freeIP, mask)
	ipclaim.Metadata.SetLabels(
		map[string]string{"ip-pool-name": pool.Metadata.Name})

	err := tryCreateIPClaim(s.ExtensionsClientset, ipclaim)
	if err != nil {
		glog.Errorf("Unable to create ip claim for IP %v. Details %v",
			freeIP, err)
	}

	err = updatePoolAllocation(s.ExtensionsClientset, &pool, freeIP, ipclaim.Metadata.Name)
	if err != nil {
		glog.Errorf("Unable to update IP pool's %v allocation. Details %v",
			pool.Metadata.Name, err)
	}

	err = addServiceExternalIP(svc, s.Clientset, freeIP)
	if err != nil {
		glog.Errorf("Unable to update ExternalIPs for service %v. Details: %v",
			svc.ObjectMeta, err)
	}
}

func (s *ipClaimScheduler) claimWatcher(stop chan struct{}) {
	store, controller := cache.NewInformer(
		s.claimSource,
		&extensions.IpClaim{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				claim := obj.(*extensions.IpClaim)
				glog.V(3).Infof("Ipclaim created %v", claim.Metadata.Name)
				key, err := cache.MetaNamespaceKeyFunc(claim)
				if err != nil {
					glog.Errorf("Error getting key %v", err)
				}
				s.queue.Add(key)
			},
			UpdateFunc: func(_, cur interface{}) {
				claim := cur.(*extensions.IpClaim)
				glog.V(3).Infof("Ipclaim updated %v, resource version %v",
					claim.Metadata.Name, claim.Metadata.ResourceVersion)
				key, err := cache.MetaNamespaceKeyFunc(claim)
				if err != nil {
					glog.Errorf("Error getting key %v", err)
				}
				s.queue.Add(key)
			},
			DeleteFunc: func(obj interface{}) {
				claim := obj.(*extensions.IpClaim)
				glog.V(3).Infof("Ipclaim deleted %v, resource version %v",
					claim.Metadata.Name, claim.Metadata.ResourceVersion)
			},
		},
	)
	s.claimStore = store
	controller.Run(stop)
}

func (s *ipClaimScheduler) findAliveNodes(ipnodes []extensions.IpNode) (result []*extensions.IpNode) {
	s.liveSync.Lock()
	s.liveSync.Unlock()
	for i := range ipnodes {
		node := ipnodes[i]
		if _, ok := s.liveIpNodes[node.Metadata.Name]; ok {
			result = append(result, &node)
		}
	}
	return result
}

func (s *ipClaimScheduler) worker() {
	glog.V(1).Infof("Starting worker to process ip claims")
	for {
		key, quit := s.queue.Get()
		glog.V(3).Infof("Got claim %v to process", key)
		if quit {
			return
		}
		item, exists, _ := s.claimStore.GetByKey(key.(string))
		if exists {
			err := s.processIpClaim(item.(*extensions.IpClaim))
			if err != nil {
				glog.Errorf("Error processing claim %v", err)
				s.queue.Add(key)
			}
		}
		glog.V(5).Infof("Processed claim %v", key)
		s.queue.Done(key)
	}
}

func (s *ipClaimScheduler) processOldService(svc *v1.Service) {
	refs := map[string]struct{}{}
	svcList := s.serviceStore.List()
	for i := range svcList {
		svc := svcList[i].(*v1.Service)
		for _, ip := range svc.Spec.ExternalIPs {
			refs[ip] = struct{}{}
		}
	}

	pools, err := s.ExtensionsClientset.IPClaimPools().List(api.ListOptions{})
	if err != nil {
		glog.Errorf("Error while retrieving list of IP pools. Details: %v", err)
	}

	for _, ip := range svc.Spec.ExternalIPs {
		if _, ok := refs[ip]; !ok {
			if p := poolAllocating(ip, pools); p != nil {
				deleteIPClaim(s.ExtensionsClientset, ip, strings.Split(p.Spec.CIDR, "/")[1])
				delete(p.Spec.Allocated, ip)
				continue
			}
			deleteIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
		}
	}
	removeAllocation(s.ExtensionsClientset, pools)
}

func removeAllocation(ext extensions.ExtensionsClientset, poolList *extensions.IpClaimPoolList) {
	glog.V(5).Infof("Try to remove allocation from the pools")
	for _, pool := range poolList.Items {
		glog.V(2).Infof("Try to update IP pool with object %v", pool)
		_, err := ext.IPClaimPools().Update(&pool)
		if err != nil {
			glog.Errorf("Unable to free allocation for pool. Details: %v",
				pool.Metadata.Name, err)
		}
	}
}

func (s *ipClaimScheduler) processIpClaim(claim *extensions.IpClaim) error {
	if claim.Spec.NodeName != "" && s.isLive(claim.Spec.NodeName) {
		return nil
	}
	ipnodes, err := s.ExtensionsClientset.IPNodes().List(api.ListOptions{})
	if err != nil {
		return err
	}
	// this needs to be queued and requeud in case of node absence
	if len(ipnodes.Items) == 0 {
		return fmt.Errorf("No nodes")
	}
	liveNodes := s.findAliveNodes(ipnodes.Items)
	if len(liveNodes) == 0 {
		return fmt.Errorf("No live nodes")
	}
	ipnode := s.findFairNode(liveNodes)
	claim.Metadata.SetLabels(map[string]string{"ipnode": ipnode.Metadata.Name})
	claim.Spec.NodeName = ipnode.Metadata.Name
	glog.V(3).Infof("Scheduling claim %v on a node %v",
		claim.Metadata.Name, claim.Spec.NodeName)
	claim, err = s.ExtensionsClientset.IPClaims().Update(claim)
	glog.V(3).Infof("Claim %v updated with node %v. Resource version %v",
		claim.Metadata.Name, claim.Spec.NodeName, claim.Metadata.ResourceVersion)
	return err
}

func (s *ipClaimScheduler) findFairNode(ipnodes []*extensions.IpNode) *extensions.IpNode {
	counter := make(map[string]int)
	for _, key := range s.claimStore.ListKeys() {
		obj, _, err := s.claimStore.GetByKey(key)
		claim := obj.(*extensions.IpClaim)
		if err != nil {
			glog.Errorln(err)
			continue
		}
		if claim.Spec.NodeName == "" {
			continue
		}
		counter[claim.Spec.NodeName]++
	}
	var min *extensions.IpNode
	minCount := -1
	for _, node := range ipnodes {
		count := counter[node.Metadata.Name]
		if minCount == -1 || count < minCount {
			minCount = count
			min = node
		}
	}
	return min
}

func (s *ipClaimScheduler) monitorIPNodes(stop chan struct{}, ticker <-chan time.Time) {
	for {
		select {
		case <-stop:
			return
		case <-ticker:
			ipnodes, err := s.ExtensionsClientset.IPNodes().List(api.ListOptions{})
			if err != nil {
				glog.Errorf("Error in monitor ip-nodes %v", err)
			}

			for _, ipnode := range ipnodes.Items {
				name := ipnode.Metadata.Name
				version := s.observedGeneration[name]
				curVersion := ipnode.Revision
				if version < curVersion {
					s.observedGeneration[name] = curVersion
					s.liveSync.Lock()
					glog.V(3).Infof("Ipnode %v is alive. Versions %v - %v",
						name, version, curVersion)
					s.liveIpNodes[name] = struct{}{}
					s.liveSync.Unlock()
				} else {
					s.liveSync.Lock()
					glog.V(3).Infof("Ipnode %v is dead. Versions %v - %v",
						name, version, curVersion)
					delete(s.liveIpNodes, name)
					s.liveSync.Unlock()
					labelSelector := labels.Set(map[string]string{"ipnode": name})
					ipclaims, err := s.ExtensionsClientset.IPClaims().List(
						api.ListOptions{
							LabelSelector: labelSelector.AsSelector(),
						},
					)
					if err != nil {
						glog.Errorf("Error fetching list of claims: %v", err)
						break
					}
					for _, ipclaim := range ipclaims.Items {
						// TODO don't update - send to queue for rescheduling instead
						glog.Infof("Sending ipclaim %v for rescheduling. CIDR %v, Previos node %v",
							ipclaim.Metadata.Name, ipclaim.Spec.Cidr, ipclaim.Spec.NodeName)
						key, err := cache.MetaNamespaceKeyFunc(&ipclaim)
						if err != nil {
							glog.Errorf("Error getting key %v", err)
						} else {
							s.queue.Add(key)
						}
					}
				}
			}
		}
	}
}

func (s *ipClaimScheduler) isLive(name string) bool {
	s.liveSync.Lock()
	defer s.liveSync.Unlock()
	_, ok := s.liveIpNodes[name]
	return ok
}

func checkAnnotation(svc *v1.Service) bool {
	if svc.ObjectMeta.Annotations != nil {
		val, exists := svc.ObjectMeta.Annotations[AutoExternalAnnotationKey]
		glog.V(5).Infof(
			"Auto allocation annotation of key '%v' provided for service %v with value %v",
			AutoExternalAnnotationKey, svc.ObjectMeta.Name, val,
		)
		if exists {
			if val == AutoExternalAnnotationValue {
				return true
			}
			glog.Warning("Only 'auto' value is processed for 'external-ip' annotation key")
		}
	}
	return false
}

func updatePoolAllocation(ext extensions.ExtensionsClientset, pool *extensions.IpClaimPool, ip, claimName string) error {
	if pool.Spec.Allocated != nil {
		pool.Spec.Allocated[ip] = claimName
	} else {
		pool.Spec.Allocated = map[string]string{ip: claimName}
	}

	glog.V(2).Infof("Update pool with object %v", pool)
	_, err := ext.IPClaimPools().Update(pool)
	if err != nil {
		return err
	}
	return nil
}

func addServiceExternalIP(svc *v1.Service, kcs kubernetes.Interface, ip string) error {
	glog.V(5).Infof(
		"Try to update service's %v external IPs list with address %v",
		svc.ObjectMeta.Name, ip,
	)
	svc.Spec.ExternalIPs = append(svc.Spec.ExternalIPs, ip)
	_, err := kcs.Core().Services(svc.ObjectMeta.Namespace).Update(svc)
	if err != nil {
		return err
	}
	return nil
}

func tryCreateIPClaim(ext extensions.ExtensionsClientset, ipclaim *extensions.IpClaim) error {
	_, err := ext.IPClaims().Create(ipclaim)
	if apierrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func makeIPClaim(ip, mask string) *extensions.IpClaim {
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")
	cidr := strings.Join([]string{ip, mask}, "/")

	glog.V(2).Infof("Creating ipclaim %v", key)

	ipclaim := &extensions.IpClaim{
		Metadata: api.ObjectMeta{Name: key},
		Spec:     extensions.IpClaimSpec{Cidr: cidr},
	}
	return ipclaim
}

func deleteIPClaim(ext extensions.ExtensionsClientset, ip, mask string) {
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")

	glog.V(2).Infof("Deleting ipclaim %v", key)
	err := ext.IPClaims().Delete(key, &api.DeleteOptions{})
	if err != nil {
		glog.Errorf("Unable to delete %v. Err %v", ip, err)
	}
}
