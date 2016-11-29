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

package app

import (
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"

	externalip "github.com/Mirantis/k8s-externalipcontroller/pkg"
)

func init() {
	Root.AddCommand(Naive)
}

var Naive = &cobra.Command{
	Aliases: []string{"n"},
	Use:     "naivecontroller",
	RunE: func(cmd *cobra.Command, args []string) error {
		return InitNaiveController()
	},
}

func InitNaiveController() error {
	kubeconfig := AppOpts.Kubeconfig
	iface := AppOpts.Iface
	mask := AppOpts.Mask

	glog.V(4).Infof("Starting external ip controller using link: %s and mask: /%s", iface, mask)
	stopCh := make(chan struct{})

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

	host, err := os.Hostname()
	if err != nil {
		return err
	}

	c, err := externalip.NewExternalIpController(config, host, iface, mask, AppOpts.ResyncInterval)
	if err != nil {
		return err
	}
	c.Run(stopCh)
	return nil
}
