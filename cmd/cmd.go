package main

import (
	"flag"
	"github.com/golang/glog"
)

func main() {
	flag.Parse()
	glog.V(4).Infof("Starting external ip controller")
}
