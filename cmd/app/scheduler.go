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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/scheduler"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/1.5/rest"
	"k8s.io/client-go/1.5/tools/clientcmd"
)

var mask string

func init() {
	Controller.Flags().StringVar(&mask, "mask", "32", "Default mask to use with external ips")
	Root.AddCommand(Scheduler)
}

var Scheduler = &cobra.Command{
	Aliases: []string{"s"},
	Use:     "scheduler",
	RunE: func(cmd *cobra.Command, args []string) error {
		return InitScheduler()
	},
}

func InitScheduler() error {
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
		os.Exit(1)
	}
	stop := make(chan struct{})
	s, err := scheduler.NewIPClaimScheduler(config, mask)
	if err != nil {
		glog.Errorf("Crashed during scheduler initialization: %v", err)
		os.Exit(2)
	}
	err = extensions.EnsureThirdPartyResourcesExist(s.Clientset)
	if err != nil {
		glog.Errorf("Crashed while initializing third party resources: %v", err)
		os.Exit(2)
	}
	s.Run(stop)
	return nil
}
