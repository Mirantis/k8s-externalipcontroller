package externalip

import (
	"reflect"

	"github.com/golang/glog"
	"github.com/vishvananda/netlink"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/cache"
)

func Run(config *rest.Config, iface string, mask string, stopCh chan struct{}) error {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return err
	}

	_, controller := cache.NewInformer(
		&cache.ListWatch{
			ListFunc: func(options api.ListOptions) (runtime.Object, error) {
				return clientset.Core().Services(api.NamespaceAll).List(api.ListOptions{})
			},
			WatchFunc: func(options api.ListOptions) (watch.Interface, error) {
				return clientset.Core().Services(api.NamespaceAll).Watch(api.ListOptions{})
			},
		},
		&v1.Service{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				processServiceExternalIPs(iface, mask, obj.(*v1.Service))
			},
			UpdateFunc: func(old, cur interface{}) {
				processServiceExternalIPs(iface, mask, cur.(*v1.Service))
			},
			DeleteFunc: func(obj interface{}) {
				// TODO implement deletion
			},
		},
	)
	controller.Run(stopCh)
	return nil
}

func processServiceExternalIPs(iface, mask string, service *v1.Service) {
	for i := range service.Spec.ExternalIPs {
		cidr := service.Spec.ExternalIPs[i] + "/" + mask
		if err := EnsureIPAssigned(iface, cidr); err != nil {
			glog.Errorf("IP: %s. ERROR: %v", cidr, err)
		} else {
			glog.V(4).Infof("IP: %s was successfully assigned", cidr)
		}
	}
}

func EnsureIPAssigned(iface, cidr string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return err
	}
	addrList, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}
	for i := range addrList {
		// maybe just compare IP
		if reflect.DeepEqual(&addrList[i], addr) {
			return nil
		}
	}
	return netlink.AddrAdd(link, addr)
}
