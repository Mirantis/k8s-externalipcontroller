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

package netutils

import (
    "errors"

	"github.com/vishvananda/netlink"
)

// EnsureIPAssigned will check if ip is already present on a given link
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

// ensure that given IP is not present on a given link
func EnsureIPUnassigned(iface, cidr string) error {
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
			return netlink.AddrDel(link, addr)
		}
	}
	return errors.New("IP address was not assigned")
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
	return EnsureIPUnassigned(iface, cidr)
}

type AddCIDR struct {
	Cidr string
}

type DelCIDR struct {
	Cidr string
}
