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
	"reflect"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	"github.com/golang/glog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/api"
	"k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type ExternalIpController struct {
	Uid   string
	Iface string
	Mask  string

	source    cache.ListerWatcher
	ipHandler netutils.IPHandler
	Queue     workqueue.QueueType

	resyncInterval time.Duration
}

func NewExternalIpController(config *rest.Config, uid, iface, mask string, resyncInterval time.Duration) (*ExternalIpController, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	lw := &cache.ListWatch{
		ListFunc: func(options metav1.ListOptions) (runtime.Object, error) {
			return clientset.Core().Services(api.NamespaceAll).List(metav1.ListOptions{})
		},
		WatchFunc: func(options metav1.ListOptions) (watch.Interface, error) {
			return clientset.Core().Services(api.NamespaceAll).Watch(metav1.ListOptions{})
		},
	}
	return &ExternalIpController{
		Uid:            uid,
		Iface:          iface,
		Mask:           mask,
		source:         lw,
		ipHandler:      netutils.LinuxIPHandler{},
		Queue:          workqueue.NewQueue(),
		resyncInterval: resyncInterval,
	}, nil
}

func NewExternalIpControllerWithSource(uid, iface, mask string, source cache.ListerWatcher) *ExternalIpController {
	return &ExternalIpController{
		Uid:       uid,
		Iface:     iface,
		Mask:      mask,
		source:    source,
		ipHandler: netutils.LinuxIPHandler{},
		Queue:     workqueue.NewQueue(),
	}
}

func (c *ExternalIpController) Run(stopCh chan struct{}) {
	glog.Infof("Starting externalipcontroller")
	var store cache.Store
	store, controller := cache.NewInformer(
		c.source,
		&v1.Service{},
		c.resyncInterval,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				c.processServiceExternalIPs(nil, obj.(*v1.Service), store)
			},
			UpdateFunc: func(old, cur interface{}) {
				c.processServiceExternalIPs(old.(*v1.Service), cur.(*v1.Service), store)
			},
			DeleteFunc: func(obj interface{}) {
				c.processServiceExternalIPs(obj.(*v1.Service), nil, store)
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
		err = c.ipHandler.Add(c.Iface, t.Cidr)
		cidr = t.Cidr
		action = "assignment"

	case *netutils.DelCIDR:
		err = c.ipHandler.Del(c.Iface, t.Cidr)
		cidr = t.Cidr
		action = "removal"
	}
	if err != nil {
		glog.Errorf("Error during %s of IP %v on %s - %v", action, cidr, c.Iface, err)
		c.Queue.Add(item)
	} else {
		glog.V(2).Infof("%s of IP %v was done successfully", action, cidr)
	}
}

func boolMapDifference(minuend, subtrahend map[string]bool) map[string]bool {
	difference := make(map[string]bool)

	for key := range minuend {
		if !subtrahend[key] {
			difference[key] = true
		}
	}

	return difference
}

func neglectIPsInUse(ips map[string]bool, key string, store cache.Store) {
	svcList := store.List()
	for s := range svcList {
		svc := svcList[s].(*v1.Service)
		svcKey, _ := cache.MetaNamespaceKeyFunc(svc)
		if svcKey != key {
			for _, ip := range svc.Spec.ExternalIPs {
				delete(ips, ip)
			}
		}
	}
}

func (c *ExternalIpController) processServiceExternalIPs(old, cur *v1.Service, store cache.Store) {
	old_ips := make(map[string]bool)
	cur_ips := make(map[string]bool)
	key := ""

	if old != nil {
		for i := range old.Spec.ExternalIPs {
			old_ips[old.Spec.ExternalIPs[i]] = true
		}
		key, _ = cache.MetaNamespaceKeyFunc(old)
	}
	if cur != nil {
		for i := range cur.Spec.ExternalIPs {
			cur_ips[cur.Spec.ExternalIPs[i]] = true
		}
		key, _ = cache.MetaNamespaceKeyFunc(cur)
	}

	if reflect.DeepEqual(cur_ips, old_ips) {
		return
	}

	ips_to_add := boolMapDifference(cur_ips, old_ips)
	ips_to_remove := boolMapDifference(old_ips, cur_ips)

	neglectIPsInUse(ips_to_add, key, store)
	neglectIPsInUse(ips_to_remove, key, store)

	for ip := range ips_to_add {
		cidr := ip + "/" + c.Mask
		c.Queue.Add(&netutils.AddCIDR{cidr})
	}
	for ip := range ips_to_remove {
		cidr := ip + "/" + c.Mask
		c.Queue.Add(&netutils.DelCIDR{cidr})
	}
}
