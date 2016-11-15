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

package naive

import (
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"

	externalip "github.com/Mirantis/k8s-externalipcontroller/pkg"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/ipmanager"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
)

var iface, mask, kubeconfig, ipmanagerType string
var etcdEndpoints []string

func init() {
	Naive.Flags().StringVar(&iface, "iface", "eth0", "Current interface will be used to assign ip addresses")
	Naive.Flags().StringVar(&mask, "mask", "32", "mask part of the cidr")
	Naive.Flags().StringVar(&kubeconfig, "kubeconfig", "", "kubeconfig to use with kubernetes client")
	Naive.Flags().StringVar(&ipmanagerType, "ipmanager", "noop", "choose noop or fair")
	Naive.Flags().StringSliceVar(&etcdEndpoints, "etcd", []string{}, "use to specify etcd endpoints")
}

var Naive = &cobra.Command{
	Aliases: []string{"n"},
	Use:     "naivecontroller",
	RunE: func(cmd *cobra.Command, args []string) error {
		return InitNaiveController()
	},
}

func InitNaiveController() error {
	glog.V(4).Infof("Starting external ip controller using link: %s and mask: /%s", iface, mask)
	stopCh := make(chan struct{})
	q := workqueue.NewQueue()

	var err error
	var config *rest.Config
	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		glog.Infof("kubeconfig is empty, assuming we are running in kubernetes cluster")
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		glog.Errorf("Error parsing config. %v", err)
		return err
	}

	var manager ipmanager.Manager
	switch ipmanagerType {
	case "fair":
		manager, err = ipmanager.NewFairEtcd(etcdEndpoints, stopCh, q)
		if err != nil {
			glog.Errorf("error intializing fair etcd manager %v", err)
			os.Exit(1)
		}
	}

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	c, err := externalip.NewExternalIpController(config, host, iface, mask, manager, q)
	if err != nil {
		return err
	}
	c.Run(stopCh)
	return nil
}
