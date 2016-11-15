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
	"github.com/Mirantis/k8s-externalipcontroller/cmd/app/naive"

	"github.com/spf13/cobra"
)

var kubeconfig string

func init() {
	Root.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Will be used for access to k8s api server")
	Root.AddCommand(naive.Naive)
}

var Root = &cobra.Command{
	Use:   "ipmanager",
	Short: "Application to manage IPs assignment to k8s services",
	Run:   func(cmd *cobra.Command, args []string) {},
}
