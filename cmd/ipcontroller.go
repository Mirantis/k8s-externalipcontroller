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

package main

import (
	"flag"
	"os"

	"github.com/golang/glog"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"

	externalip "github.com/Mirantis/k8s-externalipcontroller/pkg"
)

type FlagArray []string

func (f *FlagArray) Set(v string) error {
	*f = append(*f, v)
	return nil
}

func main() {
	iface := flag.String("iface", "eth0", "Link where ips will be assigned")
	mask := flag.String("mask", "32", "mask part of the cidr")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig to use with kubernetes client")
	ipmanagerType := flag.String("ipmanager", "noop", "choose noop or fair")
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

	host, err := os.Hostname()
	if err != nil {
		glog.Errorf("Failed to get hostname: %v", err)
		os.Exit(1)
	}

	c, err := externalip.NewExternalIpController(config, host, *iface, *mask, nil)
	if err != nil {
		glog.Errorf("Controller crashed with %v\n", err)
		os.Exit(1)
	}
	c.Run(stopCh)
	os.Exit(0)
}
