package main

import (
	"flag"
	"os"

	"github.com/golang/glog"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"

	externalip "github.com/Mirantis/k8s-externalipcontroller/pkg"
)

func main() {
	iface := flag.String("iface", "eth0", "Link where ips will be assigned")
	mask := flag.String("mask", "32", "mask part of the cidr")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig to use with kubernetes client")
	flag.Parse()

	glog.V(4).Infof("Starting external ip controller using link: %s and mask: /%s", *iface, *mask)
	stopCh := make(chan struct{})

	var err error
	var config *rest.Config
	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		glog.Infof("kubeconfig is empty, assuming we are running in kubernetes cluster")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Errorf("Error parsing config. %v", err)
		os.Exit(1)
	}

	c, err := externalip.NewExternalIpController(config, *iface, *mask)
	if err != nil {
		glog.Errorf("Controller crashed with %v\n", err)
		os.Exit(1)
	}
	c.Run(stopCh)
	os.Exit(0)
}
