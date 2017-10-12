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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/scheduler"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubernetes/pkg/api"
	clientset "k8s.io/kubernetes/pkg/client/clientset_generated/internalclientset"
	"k8s.io/kubernetes/pkg/client/leaderelection"
	"k8s.io/kubernetes/pkg/client/leaderelection/resourcelock"
	"k8s.io/kubernetes/pkg/client/record"
	"k8s.io/kubernetes/pkg/client/restclient"
)

func init() {
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
	kubeconfig := AppOpts.Kubeconfig
	mask := AppOpts.Mask
	config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		glog.Errorf("Error parsing config. %v", err)
		os.Exit(1)
	}
	stop := make(chan struct{})
	s, err := scheduler.NewIPClaimScheduler(config, mask, AppOpts.MonitorInterval, AppOpts.NodeFilter)
	if err != nil {
		glog.Errorf("Crashed during scheduler initialization: %v", err)
		os.Exit(2)
	}
	err = extensions.EnsureCRDsExist(s.Clientset)
	if err != nil {
		glog.Fatalf("Crashed while initializing third party resources: %v", err)
	}
	err = extensions.WaitCRDsEstablished(s.Clientset, 10*time.Second)
	if err != nil {
		glog.Fatalf("URLs for tprs are not registered: %v", err)
	}

	if !AppOpts.LeaderElection.LeaderElect {
		s.Run(stop)
		os.Exit(0)
	}
	glog.V(0).Infof("Running with leader election turned on.")
	run := func(_ <-chan struct{}) {
		s.Run(stop)
	}

	id, err := os.Hostname()
	if err != nil {
		glog.Fatalf("Cannot get hostname %v", err)
	}

	glog.V(0).Infof("Starting scheduler in leader election mode with id=%v", id)
	kubernetesConfig, err := restclient.InClusterConfig()
	if err != nil {
		glog.Fatalf("Error creating config %v", err)
	}
	leaderElectionClient, err := clientset.NewForConfig(restclient.AddUserAgent(kubernetesConfig, "leader-election"))
	if err != nil {
		glog.Fatalf("Incorrect configuration %v", err)
	}
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(api.EventSource{Component: "ipclaim-scheduler"})

	rl := resourcelock.EndpointsLock{
		EndpointsMeta: api.ObjectMeta{
			Namespace: "kube-system",
			Name:      "ipclaim-scheduler",
		},
		Client: leaderElectionClient,
		LockConfig: resourcelock.ResourceLockConfig{
			Identity:      id,
			EventRecorder: recorder,
		},
	}

	leaderelection.RunOrDie(leaderelection.LeaderElectionConfig{
		Lock:          &rl,
		LeaseDuration: AppOpts.LeaderElection.LeaseDuration.Duration,
		RenewDeadline: AppOpts.LeaderElection.RenewDeadline.Duration,
		RetryPeriod:   AppOpts.LeaderElection.RetryPeriod.Duration,
		Callbacks: leaderelection.LeaderCallbacks{
			OnStartedLeading: run,
			OnStoppedLeading: func() {
				glog.Fatalf("lost master")
			},
		},
	})
	return nil
}
