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
	Iface string
	Mask  string

	source    cache.ListerWatcher
	ipHandler netutils.IPHandler
	queue     workqueue.QueueType
}

func NewExternalIpController(config *rest.Config, iface, mask string) (*ExternalIpController, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
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
		Iface:     iface,
		Mask:      mask,
		source:    lw,
		ipHandler: netutils.LinuxIPHandler{},
		queue:     workqueue.NewQueue(),
	}, nil
}

func NewExternalIpControllerWithSource(iface, mask string, source cache.ListerWatcher) *ExternalIpController {
	return &ExternalIpController{
		Iface:     iface,
		Mask:      mask,
		source:    source,
		ipHandler: netutils.LinuxIPHandler{},
		queue:     workqueue.NewQueue(),
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
	c.queue.Close()
}

func (c *ExternalIpController) worker() {
	for {
		item, quit := c.queue.Get()
		if quit {
			return
		}
		var err error
		var cidr string
		var action string
		switch t := item.(type) {
		case *netutils.AddCIDR:
			err = c.ipHandler.Add(c.Iface, t.Cidr)
			cidr = t.Cidr
			action = "Assignment"
		case *netutils.DelCIDR:
			err = c.ipHandler.Del(c.Iface, t.Cidr)
			cidr = t.Cidr
			action = "Cleanup"
		}
		if err != nil {
			glog.Errorf("%v of IP %v is failed: %v", action, cidr, err)
			c.queue.Add(item)
		} else {
			glog.V(2).Infof("%v of IP %v is done successfully", action, cidr)
		}
		c.queue.Done(item)
	}
}

func (c *ExternalIpController) processServiceExternalIPs(service *v1.Service) {
	for i := range service.Spec.ExternalIPs {
		cidr := service.Spec.ExternalIPs[i] + "/" + c.Mask
		c.queue.Add(&netutils.AddCIDR{cidr})
	}
}

func (c *ExternalIpController) deleteServiceExternalIPs(service *v1.Service, store cache.Store) {
	if len(service.Spec.ExternalIPs) == 0 {
		return
	}
	ips := make(map[string]bool)
	// collect external IPs of existing services
	for s := range store.List() {
		if store.List()[s].(*v1.Service).ObjectMeta.UID != service.ObjectMeta.UID {
			for i := range store.List()[s].(*v1.Service).Spec.ExternalIPs {
				ips[store.List()[s].(*v1.Service).Spec.ExternalIPs[i]] = true
			}
		}
	}
	for _, ip := range service.Spec.ExternalIPs {
	    if _, present := ips[ip]; !present {
			cidr := ip + "/" + c.Mask
			c.queue.Add(&netutils.DelCIDR{cidr})
		}
	}
}
