package netutils

import (
	"fmt"

	"github.com/vishvananda/netlink"
)

// EnsureIPAssigned will check if ip is alrady present on a given link
func EnsureIPAssigned(iface, cidr string) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return err
	}
	addr, err := netlink.ParseAddr(cidr)
	if err != nil {
		return err
	}
	addrList, err := netlink.AddrList(link, netlink.FAMILY_ALL)
	if err != nil {
		return err
	}
	for i := range addrList {
		if addrList[i].IPNet.String() == addr.IPNet.String() {
			return nil
		}
	}
	return netlink.AddrAdd(link, addr)
}

type IPHandler interface {
	Add(iface, cidr string) error
	Del(iface, cidr string) error
}

type LinuxIPHandler struct{}

func (l LinuxIPHandler) Add(iface, cidr string) error {
	return EnsureIPAssigned(iface, cidr)
}
func (l LinuxIPHandler) Del(iface, cidr string) error {
	return fmt.Errorf("Not implemented")
}

type AddCIDR struct {
	Cidr string
}

type DelCIDR struct {
	Cidr string
}
