package main

import (
	"flag"
	"os"

	externalip "github.com/dshulyak/externalipcontroller/pkg"
	"github.com/golang/glog"
)

func main() {
	var iface string
	flag.StringVar(&iface, "iface", "eth0", "Link where ips will be assigned")
	flag.Parse()
	glog.V(4).Infof("Starting external ip controller")
	stopCh := make(chan struct{})

	if err := externalip.Run(iface, stopCh); err != nil {
		glog.Errorf("Controller crashed with %v\n", err)
		os.Exit(1)
	}
}
