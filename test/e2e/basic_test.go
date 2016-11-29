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
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	testutils "github.com/Mirantis/k8s-externalipcontroller/test/e2e/utils"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
	"k8s.io/client-go/1.5/pkg/labels"
	"k8s.io/client-go/1.5/pkg/util/intstr"
	"k8s.io/client-go/1.5/pkg/watch"
)

var _ = Describe("Basic", func() {
	var clientset *kubernetes.Clientset
	var pods []*v1.Pod
	var ns *v1.Namespace
	var addrToClear []string
	var ipcontrollerName = "externalipcontroller"
	var linkToUse = "docker0"

	BeforeEach(func() {
		var err error
		clientset, err = testutils.KubeClient()
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
		ipcontroller, err := clientset.Core().Pods(ns.Name).Get(ipcontrollerName)
		if err != nil {
			ensureAddrRemoved(clientset, *ipcontroller, linkToUse, addrToClear)
		}

		podList, _ := clientset.Core().Pods(ns.Name).List(api.ListOptions{LabelSelector: labels.Everything()})
		if CurrentGinkgoTestDescription().Failed {
			testutils.DumpLogs(clientset, podList.Items...)
		}
		for _, pod := range podList.Items {
			clientset.Core().Pods(pod.Namespace).Delete(pod.Name, &api.DeleteOptions{})
		}
		clientset.Namespaces().Delete(ns.Name, &api.DeleteOptions{})
	})

	It("Service should be reachable using assigned external ips [pod-version]", func() {
		By("deploying externalipcontroller pod")
		// TODO make docker0 iface configurable
		externalipcontroller := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "n", "--logtostderr=true", "--v=10", "--iface=" + linkToUse, "--mask=24"}, nil, true, true)
		pod, err := clientset.Pods(ns.Name).Create(externalipcontroller)
		pods = append(pods, pod)
		Expect(err).Should(BeNil())
		testutils.WaitForReady(clientset, pod)

		By("deploying nginx pod application and service with extnernal ips")
		externalIPs := []string{"10.108.10.3"}
		addrToClear = append(addrToClear, externalIPs...)
		nginxName := "nginx"
		var nginxPort int32 = 2288
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.108.10.4/24")).Should(BeNil())

		By("veryfiying that service is reachable using external ip")
		verifyServiceReachable(nginxPort, externalIPs...)
	})
})

var _ = Describe("Third party objects", func() {
	var clientset *kubernetes.Clientset
	var ext extensions.ExtensionsClientset
	var daemonSets []*v1beta1.DaemonSet
	var ns *v1.Namespace
	var addrToClear []string
	var ipcontrollerLabels = map[string]string{"app": "ipcontroller"}
	var linkToUse = "docker0"
	var nodes *v1.NodeList

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
		By("adding name=<name> label to each node")
		nodes, err = clientset.Core().Nodes().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, node := range nodes.Items {
			node.Labels["name"] = node.Name
			_, err := clientset.Core().Nodes().Update(&node)
			Expect(err).NotTo(HaveOccurred())
		}
	})

	AfterEach(func() {
		selector := labels.Set(ipcontrollerLabels).AsSelector()
		ipcontrollerPods, err := clientset.Core().Pods(ns.Name).List(
			api.ListOptions{LabelSelector: selector},
		)
		if err != nil {
			for _, pod := range ipcontrollerPods.Items {
				ensureAddrRemoved(clientset, pod, linkToUse, addrToClear)
			}
		}

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
			ext.IPNodes().Delete(item.Metadata.Name, &api.DeleteOptions{})
		}

		ipclaims, err := ext.IPClaims().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, item := range ipclaims.Items {
			ext.IPClaims().Delete(item.Metadata.Name, &api.DeleteOptions{})
		}

		ipclaimpools, err := ext.IPClaimPools().List(api.ListOptions{})
		Expect(err).NotTo(HaveOccurred())
		for _, item := range ipclaimpools.Items {
			ext.IPClaimPools().Delete(item.Metadata.Name, &api.DeleteOptions{})
		}
	})

	It("Scheduler will correctly handle creation/deletion of ipclaims based on externalips [Native]", func() {
		By("deploying scheduler pod")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "s", "--mask=24", "--logtostderr", "--v=10"}, nil, false, false)
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
			if len(ipclaims.Items) >= 2 {
				return fmt.Errorf("Expected to see atleast 2 ipclaims, instead %v", ipclaims.Items)
			}
			return nil
		}, 30*time.Second, 2*time.Second).Should(BeNil())
	})

	It("Create IP claim pool resource via its client and try to retrieve it from k8s api back", func() {
		var err error
		err = extensions.EnsureThirdPartyResourcesExist(clientset)
		Expect(err).NotTo(HaveOccurred())
		ipclaimpool := &extensions.IpClaimPool{
			Metadata: api.ObjectMeta{Name: "testclaimpool"},
			Spec: extensions.IpClaimPoolSpec{
				Range: "10.20.0.0/24",
			},
		}
		_, err = ext.IPClaimPools().Create(ipclaimpool)
		Expect(err).NotTo(HaveOccurred())

		created, err := ext.IPClaimPools().Get("testclaimpool")
		Expect(err).NotTo(HaveOccurred())
		Expect(created.Spec.Range).To(Equal("10.20.0.0/24"))
		Expect(created.Metadata.Name).To(Equal("testclaimpool"))
	})

	It("IpClaim watcher should work with resouce version as expected", func() {
		By("ensuring that third party resources are created")
		err := extensions.EnsureThirdPartyResourcesExist(clientset)
		By("creating ipclaim object")
		Expect(err).NotTo(HaveOccurred())
		ipclaim := &extensions.IpClaim{
			Metadata: api.ObjectMeta{
				Name:   "watchclaim",
				Labels: map[string]string{"ipnode": "test"}},
			Spec: extensions.IpClaimSpec{
				Cidr: "10.10.0.2/24",
			},
		}
		first, err := ext.IPClaims().Create(ipclaim)
		Expect(err).NotTo(HaveOccurred())
		testutils.Logf("first resource version %v\n", first.Metadata.ResourceVersion)
		By("creating another ipclaim")
		ipclaim.Metadata.Name = "anothertest"
		second, err := ext.IPClaims().Create(ipclaim)
		testutils.Logf("second resouce version %v\n", second.Metadata.ResourceVersion)
		Expect(err).NotTo(HaveOccurred())
		By("creating watcher and expecting two events")
		watcher, err := ext.IPClaims().Watch(api.ListOptions{})
		defer watcher.Stop()
		Expect(err).NotTo(HaveOccurred())
		verifyEventsCount(watcher, 2)
		By("creating watcher with resource version and expecting one event")
		versionWatcher, err := ext.IPClaims().Watch(api.ListOptions{
			ResourceVersion: first.Metadata.ResourceVersion,
		})
		defer versionWatcher.Stop()
		Expect(err).NotTo(HaveOccurred())
		err = ext.IPClaims().Delete(second.Metadata.Name, &api.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
		verifyEventsCount(versionWatcher, 2)
	})

	It("Controller will add ips assigned by ipclaim [Native]", func() {
		By("Deploying controller with custom hostname")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "c", "--iface=eth0", "--hostname=test", "--logtostderr", "--v=10"},
			nil, false, true)
		pod, err := clientset.Core().Pods(ns.Name).Create(pod)
		testutils.WaitForReady(clientset, pod)
		Expect(err).NotTo(HaveOccurred())

		By("creating ipclaim that is not assigned to any host")
		fake := &extensions.IpClaim{
			Metadata: api.ObjectMeta{Name: "fakeclaim"},
		}
		_, err = ext.IPClaims().Create(fake)
		Expect(err).NotTo(HaveOccurred())

		By("creating ipclaim assigned to host with name test")
		claimLabels := map[string]string{"ipnode": "test"}
		ipclaim := &extensions.IpClaim{
			Metadata: api.ObjectMeta{
				Name:   "testclaim",
				Labels: map[string]string{"ipnode": "test"}},
			Spec: extensions.IpClaimSpec{
				Cidr:     "10.10.0.2/24",
				NodeName: "test"},
		}
		_, err = ext.IPClaims().Create(ipclaim)
		Expect(err).NotTo(HaveOccurred())

		By("verify that label selector works")
		ipclaims, err := ext.IPClaims().List(api.ListOptions{
			LabelSelector: labels.Set(claimLabels).AsSelector(),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(len(ipclaims.Items)).To(BeNumerically("==", 1))

		By("verifying that cidr provided in ip claim was assigned")
		_, network, err := net.ParseCIDR("10.10.0.0/24")
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			if ips := getManagedIps(clientset, *pod, network, "eth0"); len(ips) == 1 {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP count - %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})

	It("Controller will resync ips removed by hand [Native]", func() {
		By("Deploying controller with resync interval 1s")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"ipmanager", "c", "--iface=eth0", "--hostname=test", "--logtostderr", "--v=10", "--resync=1s"},
			nil, false, true)
		pod, err := clientset.Core().Pods(ns.Name).Create(pod)
		testutils.WaitForReady(clientset, pod)
		Expect(err).NotTo(HaveOccurred())

		By("creating ipclaim assigned to host with name test")
		ipclaim := &extensions.IpClaim{
			Metadata: api.ObjectMeta{
				Name:   "testclaim",
				Labels: map[string]string{"ipnode": "test"}},
			Spec: extensions.IpClaimSpec{
				Cidr:     "10.101.0.7/24",
				NodeName: "test"},
		}
		_, err = ext.IPClaims().Create(ipclaim)
		Expect(err).NotTo(HaveOccurred())

		By("verifying that cidr provided in ip claim was assigned")
		_, network, err := net.ParseCIDR("10.101.0.0/24")
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			if ips := getManagedIps(clientset, *pod, network, "eth0"); len(ips) == 1 {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP count - %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("removing " + ipclaim.Spec.Cidr + " from ipcontroller")
		ensureAddrRemoved(clientset, *pod, "eth0", []string{ipclaim.Spec.Cidr})

		By("verifying that address list is still the same")
		Eventually(func() error {
			if ips := getManagedIps(clientset, *pod, network, "eth0"); len(ips) == 1 {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP count - %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})

	It("Daemon set version should run on multiple nodes, split ips evenly and tolerate failures [Native]", func() {
		processName := "ipmanager"
		By("deploying claim scheduler pod")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{processName, "s", "--mask=24", "--logtostderr", "--v=5"}, nil, false, false)
		_, err := clientset.Core().Pods(ns.Name).Create(pod)
		Expect(err).NotTo(HaveOccurred())

		By("deploying claim controller daemon set")
		// sh -c will be PID 1 and we will be able to stop our process
		cmd := []string{processName, "c", "--logtostderr", "--v=5", "--iface=docker0"}
		ds := newDaemonSet("externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"sh", "-c", strings.Join(cmd, " ")}, ipcontrollerLabels, true, true)
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Create(ds)
		Expect(err).NotTo(HaveOccurred())

		By("waiting until both nodes will be registered")
		Eventually(func() error {
			ipnodes, err := ext.IPNodes().List(api.ListOptions{})
			if err != nil {
				return err
			}
			if len(ipnodes.Items) != 2 {
				return fmt.Errorf("Unexpected nodes length %v", ipnodes.Items)
			}
			return nil
		}, time.Second*30, 2*time.Second).Should(BeNil())

		By("deploying nginx pod and service with multiple external ips")
		nginxName := "nginx"
		var nginxPort int32 = 2288
		_, network, err := net.ParseCIDR("10.107.10.0/24")
		Expect(err).NotTo(HaveOccurred())
		externalIPs := []string{"10.107.10.2", "10.107.10.3", "10.107.10.4", "10.107.10.5"}
		addrToClear = append(addrToClear, externalIPs...)
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.107.10.10/24")).Should(BeNil())

		By("verifying that nginx service reachable using any externalIP")
		verifyServiceReachable(nginxPort, externalIPs...)

		By("verifying that ips are distributed among all daemon set pods")
		dsPods := getPodsByLabels(clientset, ns, ipcontrollerLabels)
		Expect(len(dsPods)).To(BeNumerically(">", 1))
		var totalCount int
		for i := range dsPods {
			managedIPs := getManagedIps(clientset, dsPods[i], network, "docker0")
			Expect(len(managedIPs)).To(BeNumerically("<", len(externalIPs)))
			totalCount += len(managedIPs)
		}
		Expect(totalCount).To(BeNumerically("==", len(externalIPs)))

		By("making one of the controllers unreachable and verifying that all ips are rescheduled onto the other pod")
		rst, _, err := testutils.ExecInPod(clientset, dsPods[0], "killall", "-19", processName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rst).To(BeEmpty())

		By("verify that all ips are reassigned to another pod")
		Eventually(func() error {
			allIPs := getManagedIps(clientset, dsPods[1], network, "docker0")
			if len(allIPs) != len(externalIPs) {
				return fmt.Errorf("Not all IPs were reassigned to another pod: %v", allIPs)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("bring back controller and verify that ips are purged")
		rst, _, err = testutils.ExecInPod(clientset, dsPods[0], "killall", "-18", processName)
		Expect(err).NotTo(HaveOccurred())
		Expect(rst).To(BeEmpty())
		Eventually(func() error {
			if ips := getManagedIps(clientset, dsPods[0], network, "docker0"); ips == nil {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("deleting service and verifying that ips are purged from second controller")
		err = clientset.Core().Services(ns.Name).Delete(nginxName, &api.DeleteOptions{})
		Expect(err).NotTo(HaveOccurred())
		Eventually(func() error {
			if ips := getManagedIps(clientset, dsPods[1], network, "docker0"); ips == nil {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})

	It("Daemon set version should recover after crash and reassign ips", func() {
		processName := "ipmanager"
		By("deploying claim scheduler pod")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{processName, "s", "--mask=24", "--logtostderr", "--v=5"}, nil, false, false)
		_, err := clientset.Core().Pods(ns.Name).Create(pod)
		Expect(err).NotTo(HaveOccurred())

		By("deploying claim controller daemon set")
		// sh -c will be PID 1 and we will be able to stop our process
		cmd := []string{processName, "c", "--logtostderr", "--v=5", "--iface=docker0"}
		ds := newDaemonSet("externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"sh", "-c", strings.Join(cmd, " ")}, ipcontrollerLabels, true, true)
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Create(ds)
		Expect(err).NotTo(HaveOccurred())

		By("waiting until both nodes will be registered")
		Eventually(func() error {
			ipnodes, err := ext.IPNodes().List(api.ListOptions{})
			if err != nil {
				return err
			}
			if len(ipnodes.Items) != 2 {
				return fmt.Errorf("Unexpected nodes length %v", ipnodes.Items)
			}
			return nil
		}, time.Second*30, 2*time.Second).Should(BeNil())

		By("deploying nginx pod and service with multiple external ips")
		nginxName := "nginx"
		var nginxPort int32 = 2288
		_, network, err := net.ParseCIDR("10.107.10.0/24")
		Expect(err).NotTo(HaveOccurred())
		externalIPs := []string{"10.107.10.7", "10.107.10.8"}
		addrToClear = append(addrToClear, externalIPs...)
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		By("assigning ip from external ip pool to a node where test is running")
		Expect(netutils.EnsureIPAssigned(testutils.GetTestLink(), "10.107.10.10/24")).Should(BeNil())

		By("verifying that ips are distributed among all daemon set pods")
		dsPods := getPodsByLabels(clientset, ns, ipcontrollerLabels)
		Expect(len(dsPods)).To(BeNumerically(">", 1))
		var totalCount int
		for i := range dsPods {
			managedIPs := getManagedIps(clientset, dsPods[i], network, "docker0")
			Expect(len(managedIPs)).To(BeNumerically("<", len(externalIPs)))
			totalCount += len(managedIPs)
		}
		Expect(totalCount).To(BeNumerically("==", len(externalIPs)))

		By("adding nodeSelector which will exclude node " + nodes.Items[0].Name)
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Get("externalipcontroller")
		Expect(err).NotTo(HaveOccurred())
		ds.Spec.Template.Spec.NodeSelector = map[string]string{
			"name": nodes.Items[0].Name,
		}
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Update(ds)
		Expect(err).NotTo(HaveOccurred())
		var zero int64 = 0
		for _, pod := range dsPods {
			err := clientset.Core().Pods(ns.Name).Delete(pod.Name, &api.DeleteOptions{
				GracePeriodSeconds: &zero,
			})
			Expect(err).NotTo(HaveOccurred())
		}

		By("waiting until only single pod will be running")
		Eventually(func() error {
			dsPods := getPodsByLabels(clientset, ns, ipcontrollerLabels)
			if len(dsPods) != 1 {
				return fmt.Errorf("Unexpected length of pods %v", len(dsPods))
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("verifying that all ips are recheduled onto live pod")
		dsPods = getPodsByLabels(clientset, ns, ipcontrollerLabels)
		liveController := dsPods[0].Name
		Expect(len(dsPods)).To(BeNumerically("==", 1))
		Eventually(func() error {
			allIPs := getManagedIps(clientset, dsPods[0], network, "docker0")
			if len(allIPs) != len(externalIPs) {
				return fmt.Errorf("Not all IPs were reassigned to another pod: %v", allIPs)
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())

		By("remove nodeSelector from daemon set and wait until both of controllers will be running")
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Get("externalipcontroller")
		Expect(err).NotTo(HaveOccurred())
		ds.Spec.Template.Spec.NodeSelector = map[string]string{}
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Update(ds)
		Expect(err).NotTo(HaveOccurred())
		var newController v1.Pod
		Eventually(func() error {
			dsPods := getPodsByLabels(clientset, ns, ipcontrollerLabels)
			if len(dsPods) != 2 {
				return fmt.Errorf("Unexpected length of pods %v", len(dsPods))
			}
			for _, pod := range dsPods {
				if pod.Name != liveController {
					newController = pod
				}
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())
		By("verifying that all ips are purged from new controller " + newController.Name)
		Eventually(func() error {
			if ips := getManagedIps(clientset, newController, network, "docker0"); ips == nil {
				return nil
			} else {
				return fmt.Errorf("Unexpected IP %v", ips)
			}
		}, 30*time.Second, 1*time.Second).Should(BeNil())
	})

	It("Daemon set version with same hostname should assign all ips on all nodes", func() {
		processName := "ipmanager"
		nodeName := "testnode"
		By("deploying claim scheduler pod")
		pod := newPod(
			"externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{processName, "s", "--mask=24", "--logtostderr", "--v=5"}, nil, false, false)
		_, err := clientset.Core().Pods(ns.Name).Create(pod)
		Expect(err).NotTo(HaveOccurred())

		By("deploying claim controller daemon set")
		// sh -c will be PID 1 and we will be able to stop our process
		cmd := []string{processName, "c", "--logtostderr", "--v=5", "--iface=eth0", "--hostname=" + nodeName}
		ds := newDaemonSet("externalipcontroller", "externalipcontroller", "mirantis/k8s-externalipcontroller",
			[]string{"sh", "-c", strings.Join(cmd, " ")}, ipcontrollerLabels, false, true)
		ds, err = clientset.Extensions().DaemonSets(ns.Name).Create(ds)
		Expect(err).NotTo(HaveOccurred())

		By("waiting until both nodes will be registered")
		Eventually(func() error {
			ipnodes, err := ext.IPNodes().List(api.ListOptions{})
			if err != nil {
				return err
			}
			for _, node := range ipnodes.Items {
				if node.Metadata.Name == nodeName {
					return nil
				}
			}
			return fmt.Errorf("Node with name %v is not found", nodeName)
		}, time.Second*30, 2*time.Second).Should(BeNil())

		By("deploying nginx pod and service with multiple external ips")
		nginxName := "nginx"
		var nginxPort int32 = 2288
		_, network, err := net.ParseCIDR("10.107.10.0/24")
		Expect(err).NotTo(HaveOccurred())
		externalIPs := []string{"10.107.10.7", "10.107.10.8"}
		addrToClear = append(addrToClear, externalIPs...)
		deployNginxPodAndService(nginxName, nginxPort, clientset, ns, externalIPs)

		dsPods := getPodsByLabels(clientset, ns, ipcontrollerLabels)
		Expect(len(dsPods)).To(BeNumerically(">", 1))
		Eventually(func() error {
			for i := range dsPods {
				managedIPs := getManagedIps(clientset, dsPods[i], network, "eth0")
				if len(managedIPs) != len(externalIPs) {
					return fmt.Errorf("Managed ips %v are not equal to expected ips for pod %v",
						managedIPs, dsPods[i].Name)
				}
			}
			return nil
		}, 30*time.Second, 1*time.Second).Should(BeNil())
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

func getManagedIps(clientset *kubernetes.Clientset, pod v1.Pod, network *net.IPNet, linkName string) []net.IP {
	rst, _, err := testutils.ExecInPod(clientset, pod, "ip", "a", "show", "dev", linkName)
	Expect(err).NotTo(HaveOccurred())
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

func ensureAddrRemoved(clientset *kubernetes.Clientset, pod v1.Pod, link string, addrToClear []string) {
	testutils.Logf("Removing addrs %v from link %v for pod %v\n", addrToClear, link, pod)
	for _, addr := range addrToClear {
		// ignore all errors
		testutils.ExecInPod(clientset, pod, "ip", "a", "del", "dev", link, addr)
	}
}

func verifyEventsCount(watcher watch.Interface, expectedCount int) {
	Eventually(func() error {
		var count int
		for ev := range watcher.ResultChan() {
			claim := ev.Object.(*extensions.IpClaim)
			testutils.Logf("Received event %v -- %v\n", ev.Type, claim.Metadata.Name)
			count++
			if count == expectedCount {
				return nil
			}
		}
		return fmt.Errorf("Channel closed and didnt receive any events")
	}, 10*time.Second, 1*time.Second).Should(BeNil())
}
