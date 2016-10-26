package networktests

import (
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	externalip "github.com/Mirantis/k8s-externalipcontroller/pkg"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Network", func() {

	var targetNS netns.NsHandle
	var originNS netns.NsHandle

	BeforeEach(func() {
		var err error
		targetNS, err = netns.New()
		Expect(err).NotTo(HaveOccurred())
		originNS, err = netns.Get()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		netns.Set(originNS)
		Expect(originNS.Close()).NotTo(HaveOccurred())
		Expect(targetNS.Close()).NotTo(HaveOccurred())
	})

	It("Multiple ips can be assigned", func() {
		link := &netlink.Dummy{netlink.LinkAttrs{Name: "test0"}}
		By("adding dummy link with name " + link.Attrs().Name)
		Expect(netlink.LinkAdd(link)).NotTo(HaveOccurred())
		cidrToAssign := []string{"10.10.0.2/24", "10.10.0.2/24", "10.10.0.3/24"}
		for _, cidr := range cidrToAssign {
			err := externalip.EnsureIPAssigned(link.Attrs().Name, cidr)
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
})
