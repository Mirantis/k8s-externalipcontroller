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
	"strings"
	"sync"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/golang/glog"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/fields"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
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
	}, nil
}

type ipClaimScheduler struct {
	Config              *rest.Config
	Clientset           *kubernetes.Clientset
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
}

func (s *ipClaimScheduler) Run(stop chan struct{}) {
	go s.serviceWatcher(stop)
	go s.claimWatcher(stop)
	go s.monitorIPNodes(stop, time.Tick(s.monitorPeriod))
	<-stop
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
				for _, ip := range svc.Spec.ExternalIPs {
					err := tryCreateIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
					if err != nil {
						glog.Errorf("Unable to create ip claim %v", err)
					}
				}
			},
			UpdateFunc: func(old, cur interface{}) {
				// handle old
				curSvc := cur.(*v1.Service)
				for _, ip := range curSvc.Spec.ExternalIPs {
					err := tryCreateIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
					if err != nil {
						glog.Errorf("Unable to create ip claim %v", err)
					}
				}
			},
			DeleteFunc: func(obj interface{}) {
				refs := map[string]struct{}{}
				svcList := s.serviceStore.List()
				for i := range svcList {
					svc := svcList[i].(*v1.Service)
					for _, ip := range svc.Spec.ExternalIPs {
						refs[ip] = struct{}{}
					}
				}
				svc := obj.(*v1.Service)
				for _, ip := range svc.Spec.ExternalIPs {
					if _, ok := refs[ip]; !ok {
						err := deleteIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
						if err != nil {
							glog.Errorf("Unable to delete %v", err)
						}
					}
				}
			},
		},
	)
	s.serviceStore = store
	controller.Run(stop)
}

func (s *ipClaimScheduler) claimWatcher(stop chan struct{}) {
	store, controller := cache.NewInformer(
		s.claimSource,
		&extensions.IpClaim{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				claim := obj.(*extensions.IpClaim)
				if claim.Spec.NodeName != "" {
					return
				}
				ipnodes, err := s.ExtensionsClientset.IPNodes().List(api.ListOptions{})
				if err != nil {
					glog.Errorf("Can't get ip-nodes list %v", err)
					return
				}
				// this needs to be queued and requeud in case of node absence
				if len(ipnodes.Items) == 0 {
					glog.Errorf("No available ip-nodes to schedule claim")
					return
				}
				ipnode := ipnodes.Items[0]
				claim.SetLabels(map[string]string{"ipnode": ipnode.Name})
				claim.Spec.NodeName = ipnode.Name
				_, err = s.ExtensionsClientset.IPClaims().Update(claim)
				if err != nil {
					glog.Errorf("Claim update error %v %v", claim, err)
				}
			},
		},
	)
	s.claimStore = store
	controller.Run(stop)
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
				generation := s.observedGeneration[ipnode.Name]
				if generation < ipnode.Generation {
					s.observedGeneration[ipnode.Name] = ipnode.Generation
					s.liveSync.Lock()
					s.liveIpNodes[ipnode.Name] = struct{}{}
					s.liveSync.Unlock()
				} else {
					s.liveSync.Lock()
					delete(s.liveIpNodes, ipnode.Name)
					s.liveSync.Unlock()
					labelSelector := labels.Set(map[string]string{"ipnode": ipnode.Name})
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
						ipclaim.Spec.NodeName = ""
						ipclaim.SetLabels(map[string]string{})
						_, err := s.ExtensionsClientset.IPClaims().Update(&ipclaim)
						if err != nil {
							glog.Errorf("Error during ipclaim update %v", err)
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

func tryCreateIPClaim(ext extensions.ExtensionsClientset, ip, mask string) error {
	// check how k8s stores keys in etcd
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")
	cidr := strings.Join([]string{ip, mask}, "/")
	ipclaim := &extensions.IpClaim{
		ObjectMeta: v1.ObjectMeta{Name: key},
		Spec:       extensions.IpClaimSpec{Cidr: cidr}}
	_, err := ext.IPClaims().Create(ipclaim)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func deleteIPClaim(ext extensions.ExtensionsClientset, ip, mask string) error {
	ipParts := strings.Split(ip, ".")
	key := strings.Join([]string{strings.Join(ipParts, "-"), mask}, "-")
	return ext.IPClaims().Delete(key, &api.DeleteOptions{})
}
