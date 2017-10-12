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
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/claimcontroller"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"

	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func init() {
	Root.AddCommand(Controller)
}

var Controller = &cobra.Command{
	Aliases: []string{"c"},
	Use:     "claimcontroller",
	RunE: func(cmd *cobra.Command, args []string) error {
		return InitController()
	},
}

func InitController() error {
	var err error
	var config *rest.Config
	kubeconfig := AppOpts.Kubeconfig
	iface := AppOpts.Iface
	hostname := AppOpts.Hostname
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return err
	}
	uid := hostname
	if hostname == "" {
		uid, err = os.Hostname()
	}
	if err != nil {
		return err
	}
	stop := make(chan struct{})
	c, err := claimcontroller.NewClaimController(iface, uid, config, AppOpts.ResyncInterval, AppOpts.HeartbeatInterval)
	if err != nil {
		return err
	}
	err = extensions.EnsureThirdPartyResourcesExist(c.Clientset)
	if err != nil {
		return err
	}
	err = extensions.WaitThirdPartyResources(c.ExtensionsClientset, 10*time.Second, 1*time.Second)
	if err != nil {
		return err
	}
	c.Run(stop)
	return nil
}
