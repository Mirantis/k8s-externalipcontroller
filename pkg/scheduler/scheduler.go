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
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/golang/glog"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
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

	claimSource := &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			return ext.IpClaims().List(options)
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			return ext.IpClaims().Watch(options)
		},
	}
	return &ipClaimScheduler{
		Config:              config,
		Clientset:           clientset,
		ExtensionsClientset: ext,
		DefaultMask:         mask,

		now:             unversioned.Now,
		livenessPeriond: 5 * time.Second,
		monitorPeriod:   3 * time.Second,
		serviceSource:   serviceSource,
		claimSource:     claimSource,
	}, nil
}

type ipClaimScheduler struct {
	Config              *rest.Config
	Clientset           *kubernetes.Clientset
	ExtensionsClientset *extensions.WrappedClientset
	DefaultMask         string

	serviceSource   cache.ListerWatcher
	claimSource     cache.ListerWatcher
	now             func() unversioned.Time
	livenessPeriond time.Duration
	monitorPeriod   time.Duration
}

func (s *ipClaimScheduler) Run(stop chan struct{}) {
	go s.serviceWatcher(stop)
	go s.claimWatcher(stop)
	go s.monitorIPNodes(stop)
	<-stop
}

func (s *ipClaimScheduler) serviceWatcher(stop chan struct{}) {
	var store cache.Store
	var controller *cache.Controller
	store, controller = cache.NewInformer(
		s.serviceSource,
		&v1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				svc := obj.(*v1.Service)
				for _, ip := range svc.Spec.ExternalIPs {
					tryCreateIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
				}
			},
			UpdateFunc: func(old, cur interface{}) {
				// handle old
				curSvc := cur.(*v1.Service)
				for _, ip := range curSvc.Spec.ExternalIPs {
					tryCreateIPClaim(s.ExtensionsClientset, ip, s.DefaultMask)
				}
			},
			DeleteFunc: func(obj interface{}) {
				refs := map[string]struct{}{}
				svcList := store.List()
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
	controller.Run(stop)
}

func (s *ipClaimScheduler) claimWatcher(stop chan struct{}) {
	_, controller := cache.NewInformer(
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
				}
				// this needs to be queued and requeud in case of node absence
				if len(ipnodes.Items) == 0 {
					glog.Errorf("No available ip-nodes to schedule claim")
				}
				claim.SetLabels(map[string]string{"ipnode": ipnodes.Items[0].Name})
				claim.Spec.NodeName = ipnodes.Items[0].Name
				_, err = s.ExtensionsClientset.IpClaims().Update(claim)
				if err != nil {
					glog.Errorf("Claim update error %v", err)
				}
			},
		},
	)
	controller.Run(stop)
}

func (s *ipClaimScheduler) monitorIPNodes(stop chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case <-time.Tick(s.monitorPeriod):
			ipnodes, err := s.ExtensionsClientset.IPNodes().List(api.ListOptions{})
			if err != nil {
				glog.Errorf("Error in monitor ip-nodes %v", err)
			}
			// TODO rework it to be not time based
			for _, ipnode := range ipnodes.Items {
				if ipnode.UpdateTimestamp.Add(s.livenessPeriond).Before(s.now().Time) {
					// requeue all claims allocated to this node
					// select claims using labels and update those with Spec.NodeName = ""
					labelSelector := labels.Set(map[string]string{"ipnode": ipnode.Name})
					claims, err := s.ExtensionsClientset.IpClaims().List(api.ListOptions{
						LabelSelector: labelSelector.AsSelector(),
					})
					if err != nil {
						glog.Errorf("Error fetching claims for node %v", err)
					}
					for _, claim := range claims.Items {
						claim.Spec.NodeName = ""
						claim.SetLabels(map[string]string{})
						// don't update just requeue claim
						_, err = s.ExtensionsClientset.IpClaims().Update(&claim)
						if err != nil {
							glog.Errorf("Error during update %v", err)
						}
					}
				}
			}
		}
	}
}

func tryCreateIPClaim(ext *extensions.WrappedClientset, ip, mask string) error {
	// check how k8s stores keys in etcd
	key := strings.Join([]string{ip, mask}, "-")
	cidr := strings.Join([]string{ip, mask}, "/")
	ipclaim := &extensions.IpClaim{
		ObjectMeta: v1.ObjectMeta{Name: key},
		Spec:       extensions.IpClaimSpec{Cidr: cidr}}
	_, err := ext.IpClaims().Create(ipclaim)
	if errors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func deleteIPClaim(ext *extensions.WrappedClientset, ip, mask string) error {
	key := strings.Join([]string{ip, mask}, "-")
	return ext.IpClaims().Delete(key, &api.DeleteOptions{})
}
