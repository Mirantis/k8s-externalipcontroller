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
	"fmt"
	"errors"
	"strings"
	"time"

	"k8s.io/kubernetes/pkg/apis/componentconfig"
	"k8s.io/kubernetes/pkg/client/leaderelection"

	"github.com/spf13/pflag"
)

type options struct {
	Hostname          string
	Iface             string
	Kubeconfig        string
	Mask              string
	NodeFilter        string

	HeartbeatInterval time.Duration
	MonitorInterval   time.Duration
	ResyncInterval    time.Duration

	LeaderElection componentconfig.LeaderElectionConfiguration
}

var AppOpts = options{}

var NodeFilters = []string{
	"fair",
	"first-alive",
}

func init() {
	AppOpts.AddFlags(pflag.CommandLine)
}

func (o *options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.Iface, "iface", "eth0", "Current interface will be used to assign ip addresses")
	fs.StringVar(&o.Mask, "mask", "32", "mask part of the cidr")
	fs.StringVar(&o.Kubeconfig, "kubeconfig", "", "kubeconfig to use with kubernetes client")
	fs.StringVar(&o.Hostname, "hostname", "", "We will use os.Hostname if none provided")
	filterList := strings.Join(NodeFilters, "|")
	fs.StringVar(&o.NodeFilter, "nodefilter", NodeFilters[0], fmt.Sprintf("Possible values: %s. We will use '%s' if none was provided.", filterList, NodeFilters[0]))
	fs.DurationVar(&o.ResyncInterval, "resync", 20*time.Second, "Time to resync state for all ips")
	fs.DurationVar(&o.HeartbeatInterval, "hb", 2*time.Second, "How often to send heartbeats from controllers?")
	fs.DurationVar(&o.MonitorInterval, "monitor", 4*time.Second, "How often to check controllers liveness?")
	o.LeaderElection = leaderelection.DefaultLeaderElectionConfiguration()
	leaderelection.BindFlags(&o.LeaderElection, fs)
}

func (o *options) CheckFlags() error {
	for _, f := range NodeFilters {
		if o.NodeFilter == f {
			return nil
		}
	}
	return errors.New("Incorrect node filter is provided")
}