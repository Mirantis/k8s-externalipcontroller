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

package utils

import (
	"flag"
	"fmt"
	"io"
	"time"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/tools/clientcmd"

	"github.com/golang/glog"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var MASTER string
var TESTLINK string

func init() {
	flag.StringVar(&MASTER, "master", "http://apiserver:8888", "apiserver address to use with restclient")
	flag.StringVar(&TESTLINK, "testlink", "eth0", "link to use on the side of tests")
}

func GetTestLink() string {
	return TESTLINK
}

func KubeClient() (*kubernetes.Clientset, error) {
	glog.Infof("Using master %v\n", MASTER)
	config, err := clientcmd.BuildConfigFromFlags(MASTER, "")
	if err != nil {
		return nil, err
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return clientset, nil
}

func WaitForReady(clientset *kubernetes.Clientset, pod *v1.Pod) {
	Eventually(func() error {
		podUpdated, err := clientset.Core().Pods(pod.Namespace).Get(pod.Name)
		if err != nil {
			return err
		}
		if podUpdated.Status.Phase != v1.PodRunning {
			return fmt.Errorf("pod %v is not running phase: %v", podUpdated.Name, podUpdated.Status.Phase)
		}
		return nil
	}, 120*time.Second, 5*time.Second).Should(BeNil())
}

func DumpLogs(clientset *kubernetes.Clientset, pods ...*v1.Pod) {
	for _, pod := range pods {
		dumpLogs(clientset, pod)
	}
}

func dumpLogs(clientset *kubernetes.Clientset, pod *v1.Pod) {
	req := clientset.Core().Pods(pod.Namespace).GetLogs(pod.Name, &v1.PodLogOptions{})
	readCloser, err := req.Stream()
	Expect(err).NotTo(HaveOccurred())
	defer readCloser.Close()
	fmt.Fprintf(GinkgoWriter, "\n Dumping logs for %v:%v \n", pod.Namespace, pod.Name)
	_, err = io.Copy(GinkgoWriter, readCloser)
	Expect(err).NotTo(HaveOccurred())
}
