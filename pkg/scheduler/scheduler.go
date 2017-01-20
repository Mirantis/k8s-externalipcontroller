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
	"errors"
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

func NewIPClaimScheduler(config *rest.Config, mask string, monitorInterval time.Duration, nodeFilter string) (*ipClaimScheduler, error) {
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
	scheduler := ipClaimScheduler{
		Config:              config,
		Clientset:           clientset,
		ExtensionsClientset: ext,
		DefaultMask:         mask,

		monitorPeriod: monitorInterval,
		serviceSource: serviceSource,
		claimSource:   claimSource,

		observedGeneration: make(map[string]int64),
		liveIpNodes:        make(map[string]struct{}),

		queue: workqueue.NewQueue(),
	}

	switch nodeFilter{
	case "fair":
		scheduler.getNode = scheduler.getFairNode
	case "first-alive":
		scheduler.getNode = scheduler.getFirstAliveNode
	default:
		return nil, errors.New("Incorrect node filter is provided")
	}

	return &scheduler, nil
}

type nodeFilter func([]*extensions.IpNode) *extensions.IpNode

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

	getNode nodeFilter

	queue workqueue.QueueType
}

func (s *ipClaimScheduler) Run(stop chan struct{}) {
	glog.V(3).Infof("Starting monitor goroutine.")
	go s.monitorIPNodes(stop, time.Tick(s.monitorPeriod))
	// let's give controllers some time to register themselves after scheduler restart
	// TODO(dshulyak) consider to run monitor both for leaders/non-leaders
	time.Sleep(s.monitorPeriod)
	glog.V(3).Infof("Starting all other worker goroutines.")
	go s.worker()
	go s.serviceWatcher(stop)
	go s.claimWatcher(stop)
	<-stop
	s.queue.Close()
}

// serviceWatcher creates/deletes IPClaim based on requirements from
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

// we must take into account that loss of events and double processing of
// service objects might occur (due the way the object cache works); thus
// auto allocation must be done only in case a service is properly annotated
// and there is no already auto allocated IP for it
func (s *ipClaimScheduler) processExternalIPs(svc *v1.Service) {
	foundAuto := false

	pools, err := s.ExtensionsClientset.IPClaimPools().List(api.ListOptions{})
	if err != nil {
		glog.Errorf("Error retrieving list of IP pools. Details: %v", err)
	}

	for _, ip := range svc.Spec.ExternalIPs {
		if p := poolByAllocatedIP(ip, pools); p != nil {
			foundAuto = true
			continue
		}
		err = tryCreateIPClaim(s.ExtensionsClientset, makeIPClaim(ip, s.DefaultMask))
		if err != nil {
			glog.Errorf("Unable to create IP claim. Details: %v", err)
		}
	}

	if annotated := checkAnnotation(svc); annotated && !foundAuto {
		s.autoAllocateExternalIP(svc, pools)
	}
}

func poolByAllocatedIP(ip string, poolList *extensions.IpClaimPoolList) *extensions.IpClaimPool {
	for _, pool := range poolList.Items {
		if _, exists := pool.Spec.Allocated[ip]; exists {
			return &pool
		}
	}
	return nil
}

func (s *ipClaimScheduler) autoAllocateExternalIP(svc *v1.Service, poolList *extensions.IpClaimPoolList) {
	glog.V(5).Infof("Try to auto allocate external IP for service '%v'", svc.ObjectMeta.Name)

	var freeIP string
	var pool extensions.IpClaimPool

	for _, p := range poolList.Items {
		ip, err := p.AvailableIP()
		if err != nil {
			glog.Errorf(
				"Fail to retrieve free IP from the pool '%v'; skipping to a next one. Details: %v",
				p.Metadata.Name, err)
			continue
		}
		freeIP = ip
		pool = p
		break
	}

	if len(freeIP) == 0 {
		glog.Errorf(
			"Fail to provide external IP for service '%v'. All pools are exhausted.",
			svc.ObjectMeta.Name)
		return
	}

	mask := strings.Split(pool.Spec.CIDR, "/")[1]

	ipclaim := makeIPClaim(freeIP, mask)
	ipclaim.Metadata.SetLabels(
		map[string]string{"ip-pool-name": pool.Metadata.Name})

	err := tryCreateIPClaim(s.ExtensionsClientset, ipclaim)
	if err != nil {
		glog.Errorf("Unable to create IP claim for IP '%v'. Details: %v",
			freeIP, err)
	}

	err = updatePoolAllocation(s.ExtensionsClientset, &pool, freeIP, ipclaim.Metadata.Name)
	if err != nil {
		glog.Errorf("Unable to update IP pool's '%v' allocation. Details: %v",
			pool.Metadata.Name, err)
	}

	err = addServiceExternalIP(svc, s.Clientset, freeIP)
	if err != nil {
		glog.Errorf("Unable to update ExternalIPs for service '%v'. Details: %v",
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
				glog.V(3).Infof("IP claim '%v' was created", claim.Metadata.Name)
				key, err := cache.MetaNamespaceKeyFunc(claim)
				if err != nil {
					glog.Errorf("Error getting key for IP claim: %v", err)
				}
				s.queue.Add(key)
			},
			UpdateFunc: func(_, cur interface{}) {
				claim := cur.(*extensions.IpClaim)
				glog.V(3).Infof("IP claim '%v' was updated. Resource version: %v",
					claim.Metadata.Name, claim.Metadata.ResourceVersion)
				key, err := cache.MetaNamespaceKeyFunc(claim)
				if err != nil {
					glog.Errorf("Error getting key for IP claim: %v", err)
				}
				s.queue.Add(key)
			},
			DeleteFunc: func(obj interface{}) {
				claim := obj.(*extensions.IpClaim)
				glog.V(3).Infof("IP claim '%v' was deleted. Resource version: %v",
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
	glog.V(1).Infof("Starting worker to process IP claims")
	for {
		key, quit := s.queue.Get()
		glog.V(3).Infof("Got IP claim '%v' to process", key)
		if quit {
			return
		}
		item, exists, _ := s.claimStore.GetByKey(key.(string))
		if exists {
			err := s.processIpClaim(item.(*extensions.IpClaim))
			if err != nil {
				glog.Errorf("Error processing IP claim: %v", err)
				s.queue.Add(key)
			}
		}
		glog.V(5).Infof("Processing of IP claim '%v' was completed", key)
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

	pools := s.getIPClaimPoolList()
	for _, ip := range svc.Spec.ExternalIPs {
		if _, ok := refs[ip]; !ok {
			s.deleteIPClaimAndAllocation(ip, pools)
		}
	}
}

func (s *ipClaimScheduler) getIPClaimPoolList () *extensions.IpClaimPoolList {
	pools, err := s.ExtensionsClientset.IPClaimPools().List(api.ListOptions{})
	if err != nil {
		glog.Errorf("Error retrieving list of IP pools. Details: %v", err)
	}
	return pools
}

func (s *ipClaimScheduler) deleteIPClaimAndAllocation (ip string, pools *extensions.IpClaimPoolList) {
	if p := poolByAllocatedIP(ip, pools); p != nil {
		deleteIPClaim(s.ExtensionsClientset, ip, strings.Split(p.Spec.CIDR, "/")[1])
		delete(p.Spec.Allocated, ip)

		glog.V(2).Infof("Try to update IP pool with object %v", p)
		_, err := s.ExtensionsClientset.IPClaimPools().Update(&p)
		if err != nil {
			glog.Errorf("Unable to update IP pool '%v'. Details: %v", p.Metadata.Name, err)
		}
	} else {
		deleteIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
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
	// this needs to be queued and requeued in case of node absence
	if len(ipnodes.Items) == 0 {
		return fmt.Errorf("No nodes")
	}
	liveNodes := s.findAliveNodes(ipnodes.Items)
	if len(liveNodes) == 0 {
		return fmt.Errorf("No live nodes")
	}
	ipnode := s.getNode(liveNodes)
	claim.Metadata.SetLabels(map[string]string{"ipnode": ipnode.Metadata.Name})
	claim.Spec.NodeName = ipnode.Metadata.Name
	glog.V(3).Infof("Scheduling IP claim '%v' on a node '%v'",
		claim.Metadata.Name, claim.Spec.NodeName)
	claim, err = s.ExtensionsClientset.IPClaims().Update(claim)
	glog.V(3).Infof("IP claim '%v' was updated with node '%v'. Resource version: %v",
		claim.Metadata.Name, claim.Spec.NodeName, claim.Metadata.ResourceVersion)
	return err
}

func (s *ipClaimScheduler) monitorIPNodes(stop chan struct{}, ticker <-chan time.Time) {
	for {
		select {
		case <-stop:
			return
		case <-ticker:
			ipnodes, err := s.ExtensionsClientset.IPNodes().List(api.ListOptions{})
			if err != nil {
				glog.Errorf("Error getting IP nodes: %v", err)
			}

			for _, ipnode := range ipnodes.Items {
				name := ipnode.Metadata.Name
				version := s.observedGeneration[name]
				curVersion := ipnode.Revision
				if version < curVersion {
					s.observedGeneration[name] = curVersion
					s.liveSync.Lock()
					glog.V(3).Infof("IP node '%v' is alive. Versions: %v - %v",
						name, version, curVersion)
					s.liveIpNodes[name] = struct{}{}
					s.liveSync.Unlock()
				} else {
					s.liveSync.Lock()
					glog.V(3).Infof("IP node '%v' is dead. Versions: %v - %v",
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
						glog.Errorf("Error fetching list of IP claims: %v", err)
						break
					}
					for _, ipclaim := range ipclaims.Items {
						// TODO don't update - send to queue for rescheduling instead
						glog.Infof("Sending IP claim '%v' for rescheduling. CIDR '%v', previous node '%v'",
							ipclaim.Metadata.Name, ipclaim.Spec.Cidr, ipclaim.Spec.NodeName)
						key, err := cache.MetaNamespaceKeyFunc(&ipclaim)
						if err != nil {
							glog.Errorf("Error getting key for IP claim: %v", err)
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
		if exists {
			glog.V(5).Infof(
				"Auto-allocation annotation (key '%v') is provided for service '%v' with value '%v'",
				AutoExternalAnnotationKey, svc.ObjectMeta.Name, val,
			)
			if val == AutoExternalAnnotationValue {
				return true
			}
			glog.Warning("Only value '%v' is processed for annotation key '%v'",
				AutoExternalAnnotationValue, AutoExternalAnnotationKey)
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

	glog.V(2).Infof("Update IP pool with object %v", pool)
	_, err := ext.IPClaimPools().Update(pool)
	return err
}

func addServiceExternalIP(svc *v1.Service, kcs kubernetes.Interface, ip string) error {
	glog.V(5).Infof(
		"Try to update externalIPs list of service '%v' with IP address '%v'",
		svc.ObjectMeta.Name, ip,
	)
	svc.Spec.ExternalIPs = append(svc.Spec.ExternalIPs, ip)
	_, err := kcs.Core().Services(svc.ObjectMeta.Namespace).Update(svc)
	return err
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

	glog.V(2).Infof("Creating IP claim '%v'", key)

	ipclaim := &extensions.IpClaim{
		Metadata: api.ObjectMeta{Name: key},
		Spec:     extensions.IpClaimSpec{Cidr: cidr},
	}
	return ipclaim
}

func deleteIPClaim(ext extensions.ExtensionsClientset, ip, mask string) {
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")

	glog.V(2).Infof("Deleting IP claim '%v'", key)
	err := ext.IPClaims().Delete(key, &api.DeleteOptions{})
	if err != nil {
		glog.Errorf("Unable to delete IP claim '%v'. Details: %v", ip, err)
	}
}
