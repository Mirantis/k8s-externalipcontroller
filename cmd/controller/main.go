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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/claimcontroller"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/golang/glog"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"
)

func main() {
	iface := flag.String("iface", "eth0", "Link where ips will be assigned")
	kubeconfig := flag.String("kubeconfig", "", "kubeconfig to use with kubernetes client")
	flag.Parse()
	var err error
	var config *rest.Config
	if *kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	} else {
		glog.Infof("kubeconfig is empty, assuming we are running in kubernetes cluster")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Errorf("Error parsing config: %v", err)
		os.Exit(1)
	}
	uid, err := os.Hostname()
	if err != nil {
		glog.Errorf("Error fetching hostname: %v", err)
	}
	stop := make(chan struct{})
	c, err := claimcontroller.NewClaimController(*iface, uid, config)
	if err != nil {
		glog.Errorf("Error creating claim controller: %v", err)
	}
	err = extensions.EnsureThirdPartyResourcesExist(c.Clientset)
	if err != nil {
		glog.Errorf("Crashed while initializing third party resources: %v", err)
		os.Exit(2)
	}
	c.Run(stop)
}
