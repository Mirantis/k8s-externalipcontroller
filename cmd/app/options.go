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
	"time"

	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/client/leaderelection"

	"github.com/spf13/pflag"
)

type options struct {
	Kubeconfig     string
	Iface          string
	Mask           string
	Hostname       string
	ResyncInterval time.Duration

	LeaderElection componentconfig.LeaderElectionConfiguration
}

var AppOpts = options{}

func init() {
	AppOpts.AddFlags(pflag.CommandLine)
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Iface, "iface", "eth0", "Current interface will be used to assign ip addresses")
	fs.StringVar(&o.Mask, "mask", "32", "mask part of the cidr")
	fs.StringVar(&o.Kubeconfig, "kubeconfig", "", "kubeconfig to use with kubernetes client")
	fs.StringVar(&o.Hostname, "hostname", "", "We will use os.Hostname if none provided")
	fs.DurationVar(&o.ResyncInterval, "resync", 20*time.Second, "Time to resync state for all ips")
	o.LeaderElection = leaderelection.DefaultLeaderElectionConfiguration()
	leaderelection.BindFlags(&o.LeaderElection, fs)
}
