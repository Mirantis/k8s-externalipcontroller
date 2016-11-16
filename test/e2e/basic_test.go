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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	testutils "github.com/Mirantis/k8s-externalipcontroller/test/e2e/utils"

	"strings"

	"net"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/util/intstr"
)

var _ = Describe("Basic", func() {
	var clientset *kubernetes.Clientset
	var ext extensions.ExtensionsClientset
	var pods []*v1.Pod
	var daemonSets []*v1beta1.DaemonSet
	var ns *v1.Namespace

	BeforeEach(func() {
		var err error
		clientset, err = testutils.KubeClient()
		Expect(err).NotTo(HaveOccurred())
		ext, err = extensions.WrapClientsetWithExtensions(clientset, testutils.LoadConfig())
		Expect(err).NotTo(HaveOccurred())
		namespaceObj := &v1.Namespace{
			ObjectMeta: v1.ObjectMeta{
				GenerateName: "e2e-tests-ipcontroller-",
				Namespace:    "",
			},
			Status: v1.NamespaceStatus{},
		}
		ns, err = clientset.Namespaces().Create(namespaceObj)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		podList, _ := clientset.Core().Pods(ns.Name).List(api.ListOptions{LabelSelector: labels.Everything()})
		if CurrentGinkgoTestDescription().Failed {
			testutils.DumpLogs(clientset, podList.Items...)
		}
		for _, pod := range podList.Items {
			clientset.Core().Pods(pod.Namespace).Delete(pod.Name, &api.DeleteOptions{})
		}
		for _, ds := range daemonSets {
			clientset.Extensions().DaemonSets(ds.Namespace).Delete(ds.Name, &api.DeleteOptions{})
		}
		clientset.Namespaces().Delete(ns.Name, &api.DeleteOptions{})
		ipnodes, err := ext.IPNodes().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, item := range ipnodes.Items {
			ext.IPNodes().Delete(item.Name, &api.DeleteOptions{})
		}
		ipclaims, err := ext.IPClaims().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, item := range ipclaims.Items {
			ext.IPClaims().Delete(item.Name, &api.DeleteOptions{})
		}
	})

	It("Service should be reachable using assigned external ips [pod-version]", func() {
		By("deploying externalipcontroller pod")
		// TODO make docker0 iface configurable
		externalipcontroller := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "n", "--logtostderr=true", "--v=10", "--iface=docker0", "--mask=24"}, nil, true, true)
		pod, err := clientset.Pods(ns.Name).Create(externalipcontroller)
		pods = append(pods, pod)
		Expect(err).Should(BeNil())
		testutils.WaitForReady(clientset, pod)

		By("deploying nginx pod application and service with extnernal ips")
		externalIPs := []string{"10.108.10.3"}
		nginxName := "nginx"
		var nginxPort int32 = 2288
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.108.10.4/24")).Should(BeNil())

		By("veryfiying that service is reachable using external ip")
		verifyServiceReachable(nginxPort, externalIPs...)
	})

	It("Daemon set version should run on multiple nodes, split ips evenly and tolerate failures [ds-version]", func() {
		processName := "ipmanager"
		By("deploying etcd pod and service")
		etcdName := "etcd"
		var etcdPort int32 = 4001
		etcdClusterIP := deployEtcdPodAndService(etcdName, etcdPort, clientset, ns)
		etcdFlag := fmt.Sprintf("-etcd=http://%s:%d", etcdClusterIP, etcdPort)

		By("deploying externalipcontroller daemon set")
		dsLabels := map[string]string{"app": "ipcontroller"}
		// sh -c will be PID and we will be able to stop our process
		cmd := []string{processName, "n", "--logtostderr=true", "--v=10",
			"--iface=docker0", "--mask=24", "--ipmanager=fair", etcdFlag}
		ds := newDaemonSet("externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"sh", "-c", strings.Join(cmd, " ")}, dsLabels, true, true)
		ds, err := clientset.Extensions().DaemonSets(ns.Name).Create(ds)
		Expect(err).NotTo(HaveOccurred())

		By("deploying nginx pod and service with multiple external ips")
		nginxName := "nginx"
		var nginxPort int32 = 2288
		_, network, err := net.ParseCIDR("10.107.10.0/24")
		Expect(err).NotTo(HaveOccurred())
		externalIPs := []string{"10.107.10.3", "10.107.10.4", "10.107.10.5"}
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.107.10.10/24")).Should(BeNil())

		By("verifying that nginx service reachable using any externalIP")
		verifyServiceReachable(nginxPort, externalIPs...)

		By("verifying that ips are distributed among all daemon set pods")
		dsPods := getPodsByLabels(clientset, ns, dsLabels)
		Expect(len(dsPods)).To(BeNumerically(">", 1))
		var totalCount int
		for i := range dsPods {
			managedIPs := getManagedIps(clientset, dsPods[i], network)
			Expect(len(managedIPs)).To(BeNumerically("<=", len(externalIPs)))
			totalCount += len(managedIPs)
		}
		Expect(totalCount).To(BeNumerically("==", len(externalIPs)))

		By("making one of the controllers unreachable and verifying that all ips are rescheduled on the other pods")
		rst := testutils.ExecInPod(clientset, dsPods[0], "pkill", "--echo", "-19", processName)
		Expect(rst).NotTo(BeEmpty())

		By("verify that all ips are reassigned to another pod")
		Eventually(func() error {
			allIPs := getManagedIps(clientset, dsPods[1], network)
			if len(allIPs) != len(externalIPs) {
				return fmt.Errorf("Not all IPs were reassigned to another pod: %v", allIPs)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("bring back controller and verify that ips are purged")
		rst = testutils.ExecInPod(clientset, dsPods[0], "pkill", "--echo", "-18", processName)
		Expect(rst).NotTo(BeEmpty())
		Eventually(func() error {
			if ips := getManagedIps(clientset, dsPods[0], network); ips == nil {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})

	It("Scheduler will correctly handle creation/deletion of ipclaims based on externalips [Native]", func() {
		By("deploying scheduler pod")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "s", "--mask=24"}, nil, false, false)
		_, err := clientset.Core().Pods(ns.Name).Create(pod)
		Expect(err).NotTo(HaveOccurred())

		By("deploying mock service with couple of external ips")
		svcPorts := []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: 9999, TargetPort: intstr.FromInt(9999)}}
		svc := newService("any", map[string]string{}, svcPorts, []string{"10.10.0.2", "10.10.0.3"})
		_, err = clientset.Core().Services(ns.Name).Create(svc)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that ipclaims created for each external ip")
		Eventually(func() error {
			ipclaims, err := ext.IPClaims().List(api.ListOptions{})
			if err != nil {
				return err
			}
			if len(ipclaims.Items) != 2 {
				return fmt.Errorf("Expected to see 2 ipclaims, instead %v", ipclaims.Items)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(BeNil())
	})

})

func newPrivilegedPodSpec(containerName, imageName string, cmd []string, hostNetwork, privileged bool) v1.PodSpec {
	return v1.PodSpec{
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
	}
}

func newPod(podName, containerName, imageName string, cmd []string, labels map[string]string, hostNetwork bool, privileged bool) *v1.Pod {
	return &v1.Pod{
		ObjectMeta: v1.ObjectMeta{
			Name:   podName,
			Labels: labels,
		},
		Spec: newPrivilegedPodSpec(containerName, imageName, cmd, hostNetwork, privileged),
	}
}

func newDaemonSet(dsName, containerName, imageName string, cmd []string, labels map[string]string, hostNetwork, privileged bool) *v1beta1.DaemonSet {
	return &v1beta1.DaemonSet{
		ObjectMeta: v1.ObjectMeta{
			Name:   dsName,
			Labels: labels,
		},
		Spec: v1beta1.DaemonSetSpec{
			Template: v1.PodTemplateSpec{
				ObjectMeta: v1.ObjectMeta{
					Labels: labels,
				},
				Spec: newPrivilegedPodSpec(containerName, imageName, cmd, hostNetwork, privileged),
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

func deployEtcdPodAndService(serviceName string, servicePort int32, clientset *kubernetes.Clientset, ns *v1.Namespace) string {
	etcdLabels := map[string]string{"app": "etcd"}
	cmd := []string{
		"/usr/local/bin/etcd",
		"--listen-client-urls=http://0.0.0.0:4001",
		"--advertise-client-urls=http://0.0.0.0:4001"}
	pod := newPod("etcd", "etcd", "gcr.io/google_containers/etcd-amd64:3.0.4", cmd, etcdLabels, false, false)
	pod, err := clientset.Core().Pods(ns.Name).Create(pod)
	Expect(err).NotTo(HaveOccurred())
	testutils.WaitForReady(clientset, pod)
	svcPorts := []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: servicePort, TargetPort: intstr.FromInt(4001)}}
	svc := newService(serviceName, etcdLabels, svcPorts, nil)
	_, err = clientset.Core().Services(ns.Name).Create(svc)
	Expect(err).NotTo(HaveOccurred())
	var clusterIP string
	Eventually(func() error {
		svc, err := clientset.Core().Services(ns.Name).Get(svc.Name)
		if err != nil {
			return err
		}
		if svc.Spec.ClusterIP == "" {
			return fmt.Errorf("cluster ip for service %v is not set", svc.Name)
		}
		clusterIP = svc.Spec.ClusterIP
		return nil
	}, 10*time.Second, 1*time.Second).Should(BeNil())
	return clusterIP
}

func deployNginxPodAndService(serviceName string, servicePort int32, clientset *kubernetes.Clientset, ns *v1.Namespace, externalIPs []string) {
	nginxLabels := map[string]string{"app": "nginx"}
	pod := newPod(
		"nginx", "nginx", "gcr.io/google_containers/nginx-slim:0.7", nil, nginxLabels, false, false)
	pod, err := clientset.Pods(ns.Name).Create(pod)
	Expect(err).Should(BeNil())
	testutils.WaitForReady(clientset, pod)

	servicePorts := []v1.ServicePort{{Protocol: v1.ProtocolTCP, Port: servicePort, TargetPort: intstr.FromInt(80)}}
	svc := newService(serviceName, nginxLabels, servicePorts, externalIPs)
	_, err = clientset.Services(ns.Name).Create(svc)
	Expect(err).Should(BeNil())
}

func verifyServiceReachable(port int32, ips ...string) {
	timeout := time.Duration(1 * time.Second)
	client := http.Client{
		Timeout: timeout,
	}
	Eventually(func() error {
		for _, ip := range ips {
			resp, err := client.Get(fmt.Sprintf("http://%s:%d", ip, port))
			if err != nil {
				return err
			}
			if resp.StatusCode > 200 {
				return fmt.Errorf("Unexpected error from nginx service: %s", resp.Status)
			}
		}
		return nil
	}, 30*time.Second, 1*time.Second).Should(BeNil())
}

func getPodsByLabels(clientset *kubernetes.Clientset, ns *v1.Namespace, podLabels map[string]string) []v1.Pod {
	selector := labels.Set(podLabels).AsSelector()
	pods, err := clientset.Pods(ns.Name).List(api.ListOptions{LabelSelector: selector})
	Expect(err).NotTo(HaveOccurred())
	return pods.Items
}

func getManagedIps(clientset *kubernetes.Clientset, pod v1.Pod, network *net.IPNet) []net.IP {
	rst := testutils.ExecInPod(clientset, pod, "ip", "a", "show", "dev", "docker0")
	Expect(rst).NotTo(BeEmpty())
	var managedIPs []net.IP
	for _, line := range strings.Split(rst, "\n") {
		if !strings.Contains(line, "inet") {
			continue
		}
		columns := strings.Fields(strings.TrimSpace(line))
		ip, _, err := net.ParseCIDR(columns[1])
		Expect(err).NotTo(HaveOccurred())
		if network.Contains(ip) {
			managedIPs = append(managedIPs, ip)
		}
	}
	return managedIPs
}
