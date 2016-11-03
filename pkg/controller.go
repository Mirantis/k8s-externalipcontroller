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
	_, controller := cache.NewInformer(
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
				// TODO implement deletion
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
		switch t := item.(type) {
		case *netutils.AddCIDR:
			err = c.ipHandler.Add(c.Iface, t.Cidr)
			cidr = t.Cidr
		case *netutils.DelCIDR:
			err = c.ipHandler.Del(c.Iface, t.Cidr)
			cidr = t.Cidr
		}
		if err != nil {
			glog.Errorf("Error assigning IP %s on %s - %v", cidr, c.Iface, err)
			c.queue.Add(item)
		} else {
			glog.V(2).Infof("IP %v was successfull assigned", cidr)
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
