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

package externalip

import (
	"github.com/Mirantis/k8s-externalipcontroller/pkg/ipmanager"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	"github.com/golang/glog"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

type ExternalIpController struct {
	Uid   string
	Iface string
	Mask  string

	source    cache.ListerWatcher
	ipHandler netutils.IPHandler
	Queue     workqueue.QueueType
	manager   ipmanager.Manager
}

func NewExternalIpController(config *rest.Config, uid, iface, mask string, ipmanagerInst ipmanager.Manager) (*ExternalIpController, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	if ipmanagerInst == nil {
		ipmanagerInst = &ipmanager.Noop{}
	}

	lw := &cache.ListWatch{
		ListFunc: func(options api.ListOptions) (runtime.Object, error) {
			return clientset.Core().Services(api.NamespaceAll).List(api.ListOptions{})
		},
		WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
			return clientset.Core().Services(api.NamespaceAll).Watch(api.ListOptions{})
		},
	}
	return &ExternalIpController{
		Uid:       uid,
		Iface:     iface,
		Mask:      mask,
		source:    lw,
		ipHandler: netutils.LinuxIPHandler{},
		Queue:     workqueue.NewQueue(),
		manager:   ipmanagerInst,
	}, nil
}

func NewExternalIpControllerWithSource(uid, iface, mask string, source cache.ListerWatcher, ipmanagerInst ipmanager.Manager) *ExternalIpController {
	if ipmanagerInst == nil {
		ipmanagerInst = &ipmanager.Noop{}
	}
	return &ExternalIpController{
		Uid:       uid,
		Iface:     iface,
		Mask:      mask,
		source:    source,
		ipHandler: netutils.LinuxIPHandler{},
		Queue:     workqueue.NewQueue(),
		manager:   ipmanagerInst,
	}
}

func (c *ExternalIpController) Run(stopCh chan struct{}) {
	glog.Infof("Starting externalipcontroller")
	var store cache.Store
	store, controller := cache.NewInformer(
		c.source,
		&v1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.processServiceExternalIPs(obj.(*v1.Service))
			},
			UpdateFunc: func(old, cur interface{}) {
				c.processServiceExternalIPs(cur.(*v1.Service))
			},
			DeleteFunc: func(obj interface{}) {
				c.deleteServiceExternalIPs(obj.(*v1.Service), store)
			},
		},
	)

	// we can spawn worker for each interface, but i doubt that we will ever need such
	// optimization
	go c.worker()
	go controller.Run(stopCh)
	<-stopCh
	c.Queue.Close()
}

func (c *ExternalIpController) worker() {
	for {
		item, quit := c.Queue.Get()
		if quit {
			return
		}
		c.processItem(item)
	}
}

func (c *ExternalIpController) processItem(item interface{}) {
	defer c.Queue.Done(item)
	var err error
	var cidr string
	var action string
	switch t := item.(type) {
	case *netutils.AddCIDR:
		fit, err := c.manager.Fit(c.Uid, t.Cidr)
		if !fit && err == nil {
			return
		}
		if err == nil {
			err = c.ipHandler.Add(c.Iface, t.Cidr)
			cidr = t.Cidr
		}
		action = "assignment"

	case *netutils.DelCIDR:
		err = c.ipHandler.Del(c.Iface, t.Cidr)
		cidr = t.Cidr
		action = "removal"
	}
	if err != nil {
		glog.Errorf("Error during IP: %s %v on %s - %v", cidr, action, c.Iface, err)
		c.Queue.Add(item)
	} else {
		glog.V(2).Infof("IP: %v %v was successfull assigned", action, cidr)
	}
}

func (c *ExternalIpController) processServiceExternalIPs(service *v1.Service) {
	for i := range service.Spec.ExternalIPs {
		cidr := service.Spec.ExternalIPs[i] + "/" + c.Mask
		c.Queue.Add(&netutils.AddCIDR{cidr})
	}
}

func (c *ExternalIpController) deleteServiceExternalIPs(service *v1.Service, store cache.Store) {
	if len(service.Spec.ExternalIPs) == 0 {
		return
	}
	ips := make(map[string]bool)
	key, _ := cache.MetaNamespaceKeyFunc(service)
	// collect external IPs of existing services
	svcList := store.List()
	for s := range svcList {
		svc := svcList[s].(*v1.Service)
		svcKey, _ := cache.MetaNamespaceKeyFunc(svc)
		if svcKey != key {
			for _, ip := range svc.Spec.ExternalIPs {
				ips[ip] = true
			}
		}
	}
	for _, ip := range service.Spec.ExternalIPs {
		if _, present := ips[ip]; !present {
			cidr := ip + "/" + c.Mask
			c.Queue.Add(&netutils.DelCIDR{cidr})
		}
	}
}
