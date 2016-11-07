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

package e2e

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	testutils "github.com/Mirantis/k8s-externalipcontroller/test/e2e/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

var _ = Describe("Basic", func() {
	var clientset *kubernetes.Clientset
	var pods []*v1.Pod

	BeforeEach(func() {
		var err error
		clientset, err = testutils.KubeClient()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if CurrentGinkgoTestDescription().Failed {
			testutils.DumpLogs(clientset, pods...)
		}
	})

	It("Service should be reachable using assigned external ips", func() {
		namespaceObj := &v1.Namespace{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: "e2e-tests-ipcontroller-",
				Namespace:    "",
			},
			Status: v1.NamespaceStatus{},
		}
		ns, err := clientset.Namespaces().Create(namespaceObj)
		Expect(err).NotTo(HaveOccurred())

		By("deploying externalipcontroller pod")
		// TODO make docker0 iface configurable
		externalipcontroller := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipcontroller", "-logtostderr=true", "-v=4", "-iface=docker0", "-mask=24"}, nil, true, true)
		pod, err := clientset.Pods(ns.Name).Create(externalipcontroller)
		pods = append(pods, pod)
		Expect(err).Should(BeNil())
		testutils.WaitForReady(clientset, pod)

		By("deploying nginx pod application and service with extnernal ips")
		nginxLabels := map[string]string{"app": "nginx"}
		nginx := newPod(
			"nginx", "nginx", "gcr.io/google_containers/nginx-slim:0.7", nil, nginxLabels, false, false)
		pod, err = clientset.Pods(ns.Name).Create(nginx)
		pods = append(pods, pod)
		Expect(err).Should(BeNil())
		testutils.WaitForReady(clientset, pod)

		servicePorts := []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 2288, TargetPort: intstr.FromInt(80)}}
		svc := newService("nginx-service", nginxLabels, servicePorts, []string{"10.108.10.3"})
		svc, err = clientset.Services(ns.Name).Create(svc)
		Expect(err).Should(BeNil())

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.108.10.4/24")).Should(BeNil())

		By("veryfiying that service is reachable using external ip")
		Eventually(func() error {
			resp, err := http.Get("http://10.108.10.3:2288/")
			if err != nil {
				return err
			}
			if resp.StatusCode > 200 {
				return fmt.Errorf("Unexpected error from nginx service: %s", resp.Status)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})
})

func newPod(podName, containerName, imageName string, cmd []string, labels map[string]string, hostNetwork bool, privileged bool) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   podName,
			Labels: labels,
		},
		Spec: v1.PodSpec{
			HostNetwork: hostNetwork,
			Containers: []v1.Container{
				{
					Name:            containerName,
					Image:           imageName,
					Command:         cmd,
					SecurityContext: &v1.SecurityContext{Privileged: &privileged},
					ImagePullPolicy: v1.PullIfNotPresent,
				},
			},
		},
	}
}

func newService(serviceName string, labels map[string]string, ports []v1.ServicePort, externalIPs []string) *v1.Service {
	return &v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name: serviceName,
		},
		Spec: v1.ServiceSpec{
			Selector:    labels,
			Type:        v1.ServiceTypeNodePort,
			Ports:       ports,
			ExternalIPs: externalIPs,
		},
	}
}
