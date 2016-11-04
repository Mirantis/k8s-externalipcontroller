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

package integration

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	controller "github.com/Mirantis/k8s-externalipcontroller/pkg"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"

	"k8s.io/client-go/1.5/pkg/api/v1"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"

	"github.com/vishvananda/netlink"

	"reflect"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Network", func() {

	var linkName string

	BeforeEach(func() {
		linkName = "test11"
		ensureLinksRemoved(linkName)
	})

	AfterEach(func() {
		ensureLinksRemoved(linkName)
	})

	It("Multiple ips can be assigned", func() {
		link := &netlink.Dummy{netlink.LinkAttrs{Name: linkName}}
		By("adding dummy link with name " + link.Attrs().Name)
		Expect(netlink.LinkAdd(link)).NotTo(HaveOccurred())
		cidrToAssign := []string{"10.10.0.2/24", "10.10.0.2/24", "10.10.0.3/24"}
		for _, cidr := range cidrToAssign {
			err := netutils.EnsureIPAssigned(link.Attrs().Name, cidr)
			Expect(err).NotTo(HaveOccurred())
		}
		addrList, err := netlink.AddrList(link, netlink.FAMILY_ALL)
		Expect(err).NotTo(HaveOccurred())
		ipSet := map[string]bool{}
		expectedIpSet := map[string]bool{"10.10.0.2/24": true, "10.10.0.3/24": true}
		for i := range addrList {
			ipSet[addrList[i].IPNet.String()] = true
		}
		Expect(expectedIpSet).To(BeEquivalentTo(ipSet))
	})

	It("Controller will create provided externalIPs", func() {
		link := &netlink.Dummy{netlink.LinkAttrs{Name: linkName}}
		By("adding link for controller")
		Expect(netlink.LinkAdd(link)).NotTo(HaveOccurred())
		Expect(netlink.LinkSetUp(link)).NotTo(HaveOccurred())

		By("creating and running controller with fake source")
		stop := make(chan struct{})
		defer close(stop)
		source := fcache.NewFakeControllerSource()
		c := controller.NewExternalIpControllerWithSource(link.Attrs().Name, "24", source)
		go c.Run(stop)

		testIps := [][]string{
			{"10.10.0.2", "10.10.0.3"},
			{"10.10.0.2", "10.10.0.3", "10.10.0.4"},
			{"10.10.0.5"},
		}
		services := map[string]*v1.Service{}
		expectedIps := map[string]bool{}
		for i, ips := range testIps {
			for _, ip := range ips {
				expectedIps[strings.Join([]string{ip, c.Mask}, "/")] = true
			}
			svc := &v1.Service{
				ObjectMeta: v1.ObjectMeta{Name: "service-" + strconv.Itoa(i)},
				Spec:       v1.ServiceSpec{ExternalIPs: ips},
			}
			services[svc.Name] = svc
			source.Add(svc)
		}
		By("waiting until ips will be assigned")
		verifyAddrs(link, expectedIps)
		By("removing service with single ip and waiting until this ip won't be on a link")
		source.Delete(services["service-2"])
		delete(expectedIps, "10.10.0.5/24")
		verifyAddrs(link, expectedIps)
		By("removing service with ips that are assigned to some other service and confirming that they are still on link")
		source.Delete(services["service-1"])
		delete(expectedIps, "10.10.0.4/24")
		verifyAddrs(link, expectedIps)
	})
})

func verifyAddrs(link netlink.Link, expectedIps map[string]bool) {
	Eventually(func() error {
		resultIps := map[string]bool{}
		addrList, err := netlink.AddrList(link, netlink.FAMILY_V4)
		if err != nil {
			return err
		}
		for _, addr := range addrList {
			resultIps[addr.IPNet.String()] = true
		}
		if !reflect.DeepEqual(expectedIps, resultIps) {
			return fmt.Errorf("Assigned ips %v are not equal to expected %v.", resultIps, expectedIps)
		}
		return nil
	}, 10*time.Second, 1*time.Second).Should(BeNil())
}

func ensureLinksRemoved(links ...string) {
	for _, l := range links {
		link, err := netlink.LinkByName(l)
		if err != nil {
			continue
		}
		netlink.LinkDel(link)
	}
}
