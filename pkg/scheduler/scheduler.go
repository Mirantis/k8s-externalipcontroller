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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
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
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return clientset.Core().Services(api.NamespaceAll).List(options)
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
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

		queue:       workqueue.NewQueue(),
		changeQueue: workqueue.NewQueue(),
	}

	switch nodeFilter {
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

	queue       workqueue.QueueType
	changeQueue workqueue.QueueType
}

func (s *ipClaimScheduler) Run(stop chan struct{}) {
	glog.V(3).Infof("Starting monitor goroutine.")
	go s.monitorIPNodes(stop, time.Tick(s.monitorPeriod))
	// let's give controllers some time to register themselves after scheduler restart
	// TODO(dshulyak) consider to run monitor both for leaders/non-leaders
	time.Sleep(s.monitorPeriod)
	glog.V(3).Infof("Starting all other worker goroutines.")
	go s.worker()
	go s.claimChangeWorker()
	go s.serviceWatcher(stop)
	go s.claimWatcher(stop)
	<-stop
	s.queue.Close()
	s.changeQueue.Close()
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

	pools, err := s.ExtensionsClientset.IPClaimPools().List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("Error retrieving list of IP pools. Details: %v", err)
	}

	glog.V(2).Infof("Processing svc %s with external ips %v", svc.Name, svc.Spec.ExternalIPs)
	for _, ip := range svc.Spec.ExternalIPs {
		glog.V(2).Infof(
			"Check IP %s of a service %s for an intersection with pools: %v",
			ip, svc.Name, pools)
		if p := poolByAllocatedIP(ip, pools); p != nil {
			foundAuto = true
			continue
		}
		s.addClaimChangeRequest(makeIPClaim(ip, s.DefaultMask, svc), cache.Added)
	}
	if foundAuto {
		return
	}

	if annotated := checkAnnotation(svc); annotated {
		s.autoAllocateExternalIP(svc, pools, false)
	} else if svc.Spec.Type == v1.ServiceTypeLoadBalancer {
		s.autoAllocateExternalIP(svc, pools, true)
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

func (s *ipClaimScheduler) autoAllocateExternalIP(svc *v1.Service, poolList *extensions.IpClaimPoolList, setLBIp bool) {
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

	ipclaim := makeIPClaim(freeIP, mask, svc)
	ipclaim.Metadata.SetLabels(
		map[string]string{"ip-pool-name": pool.Metadata.Name})

	s.addClaimChangeRequest(ipclaim, cache.Added)

	err := updatePoolAllocation(s.ExtensionsClientset, &pool, freeIP, ipclaim.Metadata.Name)
	if err != nil {
		glog.Errorf("Unable to update IP pool's '%v' allocation. Details: %v",
			pool.Metadata.Name, err)
	}
	glog.V(5).Infof(
		"Try to update externalIPs list of service '%v' with IP address '%v'",
		svc.ObjectMeta.Name, freeIP,
	)
	svc.Spec.ExternalIPs = append(svc.Spec.ExternalIPs, freeIP)
	svc, err = s.Clientset.Core().Services(svc.ObjectMeta.Namespace).Update(svc)
	if err != nil {
		glog.Errorf("Unable to update ExternalIPs for service '%v'. Details: %v",
			svc.ObjectMeta, err)
	}
	if setLBIp {
		svc.Status.LoadBalancer = v1.LoadBalancerStatus{Ingress: []v1.LoadBalancerIngress{{IP: freeIP}}}
		_, err = s.Clientset.Core().Services(svc.ObjectMeta.Namespace).UpdateStatus(svc)
		if err != nil {
			glog.Errorf("Error updating service %s status with LoadBalancer IP: %v", svc.Name, err)
		}
	}

}

func (s *ipClaimScheduler) claimWatcher(stop chan struct{}) {
	store, controller := cache.NewInformer(
		s.claimSource,
		&extensions.IpClaim{},
		10*time.Second,
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

func (s *ipClaimScheduler) claimChangeWorker() {
	client := s.ExtensionsClientset.IPClaims()
	for {
		req, quit := s.changeQueue.Get()
		glog.V(3).Infof("Got IP claim change request '%v' to process", req)
		if quit {
			return
		}
		changeReq := req.(*cache.Delta)
		claim := changeReq.Object.(*extensions.IpClaim)
		switch changeReq.Type {
		case cache.Added:
			_, err := client.Create(claim)
			if apierrors.IsAlreadyExists(err) {
				// Let's add new owner ref to the owner ref list of the existing IP claim
				glog.V(3).Infof("IP claim '%v' exists already", claim.Metadata.Name)
				existing, err := client.Get(claim.Metadata.Name)
				if err != nil {
					glog.Errorf("Unable to get IP claim '%v'. Details: %v", claim.Metadata.Name, err)
					s.changeQueue.Add(changeReq)
				}
				newOwnerRef := claim.Metadata.OwnerReferences[0]
				existOwnerRefs := existing.Metadata.OwnerReferences
				alreadyThere := false
				for r := range existOwnerRefs {
					if newOwnerRef.UID == existOwnerRefs[r].UID {
						glog.V(5).Infof("Service '%v' is referenced in IP claim '%v' already",
							newOwnerRef.UID, claim.Metadata.Name)
						alreadyThere = true
						break
					}
				}
				if !alreadyThere {
					existing.Metadata.OwnerReferences = append(existOwnerRefs, newOwnerRef)
					s.addClaimChangeRequest(existing, cache.Updated)
					glog.V(3).Infof("IP claim '%v' is to be updated with reference to service '%v'",
						claim.Metadata.Name, newOwnerRef.UID)
				}
			} else if err == nil {
				glog.V(3).Infof("IP claim '%v' was created", claim.Metadata.Name)
			} else {
				glog.Errorf("Unable to create IP claim '%v'. Details: %v", claim.Metadata.Name, err)
			}
		case cache.Updated:
			_, err := client.Update(claim)
			if err == nil {
				glog.V(3).Infof("IP claim '%v' was updated with node '%v'. Resource version: %v",
					claim.Metadata.Name, claim.Spec.NodeName, claim.Metadata.ResourceVersion)
			} else {
				glog.Errorf("Unable to update IP claim '%v'. Details: %v", claim.Metadata.Name, err)
			}
		case cache.Deleted:
			err := client.Delete(claim.Metadata.Name, &metav1.DeleteOptions{})
			if err != nil {
				glog.Errorf("Unable to delete IP claim '%v'. Details: %v", claim.Metadata.Name, err)
			}
		}
		glog.V(3).Infof("Processing of IP claim '%v' change request was completed", claim.Metadata.Name)
		s.changeQueue.Done(req)
	}
}

func (s *ipClaimScheduler) addClaimChangeRequest(claim *extensions.IpClaim, change cache.DeltaType) {
	req := &cache.Delta{
		Object: claim,
		Type:   change,
	}
	s.changeQueue.Add(req)
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

func (s *ipClaimScheduler) getIPClaimByIP(ip string, pools *extensions.IpClaimPoolList) (*extensions.IpClaim, error) {
	mask := ""
	if p := poolByAllocatedIP(ip, pools); p != nil {
		mask = strings.Split(p.Spec.CIDR, "/")[1]
	} else {
		mask = s.DefaultMask
	}
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")
	return s.ExtensionsClientset.IPClaims().Get(key)
}

func (s *ipClaimScheduler) getIPClaimPoolList() *extensions.IpClaimPoolList {
	pools, err := s.ExtensionsClientset.IPClaimPools().List(metav1.ListOptions{})
	if err != nil {
		glog.Errorf("Error retrieving list of IP pools. Details: %v", err)
	}
	return pools
}

func (s *ipClaimScheduler) deleteIPClaimAndAllocation(ip string, pools *extensions.IpClaimPoolList) {
	if p := poolByAllocatedIP(ip, pools); p != nil {
		s.addClaimChangeRequest(makeIPClaim(ip, strings.Split(p.Spec.CIDR, "/")[1], nil), cache.Deleted)
		delete(p.Spec.Allocated, ip)

		glog.V(2).Infof("Try to update IP pool with object %v", p)
		_, err := s.ExtensionsClientset.IPClaimPools().Update(p)
		if err != nil {
			glog.Errorf("Unable to update IP pool '%v'. Details: %v", p.Metadata.Name, err)
		}
	} else {
		s.addClaimChangeRequest(makeIPClaim(ip, s.DefaultMask, nil), cache.Deleted)
	}
}

// returns list of owner references that are relevant at the moment
func (s *ipClaimScheduler) ownersAlive(claim *extensions.IpClaim) []metav1.OwnerReference {
	// only services can be the claim owners for now
	owners := []metav1.OwnerReference{}
	for _, owner := range claim.Metadata.OwnerReferences {
		_, exists, err := s.serviceStore.GetByKey(string(owner.UID))
		if err != nil {
			glog.Errorf("Checking claim '%v' owners: error getting service '%v' from cache: %v", claim.Metadata.Name, owner.UID, err)
		}
		if !exists {
			glog.V(5).Infof("Checking claim '%v' owners: service '%v' is not in cache", claim.Metadata.Name, owner.UID)
			ns_name := strings.Split(string(owner.UID), "/")
			// "an empty namespace may not be set when a resource name is provided" error is thrown when
			// calling Services.Get w/o a namespace
			if len(ns_name) == 2 {
				_, err = s.Clientset.Core().Services(ns_name[0]).Get(ns_name[1], metav1.GetOptions{})
			} else {
				glog.Errorf("Checking claim '%v' owners: cannot get namespace for service '%v'", claim.Metadata.Name, owner.UID)
			}
			if apierrors.IsNotFound(err) {
				glog.V(5).Infof("Checking claim '%v' owners: service '%v' does not exist", claim.Metadata.Name, owner.UID)
				continue
			}
			if err != nil {
				glog.Errorf("Checking claim '%v' owners: service '%v' get error: %v", claim.Metadata.Name, owner.UID, err)
			}
		}
		owners = append(owners, owner)
	}
	glog.V(5).Infof("Checking claim '%v' owners: %v", claim.Metadata.Name, owners)
	return owners
}

func (s *ipClaimScheduler) processIpClaim(claim *extensions.IpClaim) error {
	ownersAlive := s.ownersAlive(claim)
	if len(ownersAlive) == 0 {
		// all owner links are irrelevant
		pools := s.getIPClaimPoolList()
		s.deleteIPClaimAndAllocation(strings.Split(claim.Spec.Cidr, "/")[0], pools)
		return nil
	} else if len(ownersAlive) < len(claim.Metadata.OwnerReferences) {
		// some owner links are irrelevant
		claim.Metadata.OwnerReferences = ownersAlive
		s.addClaimChangeRequest(claim, cache.Updated)
	}

	if claim.Spec.NodeName != "" && s.isLive(claim.Spec.NodeName) {
		return nil
	}
	ipnodes, err := s.ExtensionsClientset.IPNodes().List(metav1.ListOptions{})
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
	s.addClaimChangeRequest(claim, cache.Updated)
	return nil
}

func (s *ipClaimScheduler) monitorIPNodes(stop chan struct{}, ticker <-chan time.Time) {
	for {
		select {
		case <-stop:
			return
		case <-ticker:
			ipnodes, err := s.ExtensionsClientset.IPNodes().List(metav1.ListOptions{})
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
						metav1.ListOptions{
							LabelSelector: labelSelector.String(),
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

func makeIPClaim(ip, mask string, svc *v1.Service) *extensions.IpClaim {
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")
	cidr := strings.Join([]string{ip, mask}, "/")

	glog.V(2).Infof("Creating IP claim '%v'", key)

	meta := metav1.ObjectMeta{Name: key}
	if svc != nil {
		svc_key, err := cache.MetaNamespaceKeyFunc(svc)
		if err != nil {
			return nil
		}
		ctrl := false
		ownerRef := metav1.OwnerReference{APIVersion: "v1", Kind: "Service", Name: svc.Name, UID: types.UID(svc_key), Controller: &ctrl}
		meta.OwnerReferences = []metav1.OwnerReference{ownerRef}
	}

	ipclaim := &extensions.IpClaim{
		Metadata: meta,
		Spec:     extensions.IpClaimSpec{Cidr: cidr},
	}
	return ipclaim
}
