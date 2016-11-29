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

package extensions

import (
	"testing"
)

func checkUsableIPs(p *IpClaimPool, expected []string, t *testing.T) {
	actualIPs, err := p.UsableIPs()
	if err != nil {
		t.Errorf("Error must not occured while calling to UsableIPs, details: %v\n", err)
	}

	if len(expected) != len(actualIPs) {
		t.Errorf("Actual ip list %v is not as expected %v", actualIPs, expected)
	}
	for index, ip := range expected {
		if ip != actualIPs[index] {
			t.Errorf("Actual IP list is not equal to expected. Expected --> %v. Actual --> %v\n", expected, actualIPs)
		}
	}

}

func checkAvailableIP(p *IpClaimPool, expected string, t *testing.T) {
	actual, err := p.AvailableIP()
	if err != nil {
		t.Errorf("Error must not occur during FreeIP() method; details --> %v\n", err)
	}

	if expected != actual {
		t.Errorf("Actual free IP %v is not as expected %v\n", actual, expected)
	}
}

func TestIpClaimPoolUsableIPsFromCIDR(t *testing.T) {
	cidr := "192.168.16.248/29"

	//all IPs from the network with given CIDR
	//except first and last one
	expectedIPs := []string{
		"192.168.16.249",
		"192.168.16.250",
		"192.168.16.251",
		"192.168.16.252",
		"192.168.16.253",
		"192.168.16.254",
	}

	ClaimPool := &IpClaimPool{
		Spec: IpClaimPoolSpec{
			CIDR:  cidr,
			Range: nil,
		},
	}

	checkUsableIPs(ClaimPool, expectedIPs, t)

}

func TestIpClaimPoolUsableIPsFromRange(t *testing.T) {
	cidr := "192.168.16.248/29"

	expectedIPs := []string{
		"192.168.16.250",
		"192.168.16.251",
		"192.168.16.252",
	}

	ClaimPool := &IpClaimPool{
		Spec: IpClaimPoolSpec{
			CIDR:  cidr,
			Range: []string{"192.168.16.250", "192.168.16.252"},
		},
	}

	checkUsableIPs(ClaimPool, expectedIPs, t)

}

func TestIpClaimPoolGetFreeIP(t *testing.T) {
	cidr := "192.168.16.248/29"
	allocated := map[string]string{
		"192.168.16.249": "test-claim-249",
		"192.168.16.252": "test-claim-252",
		"192.168.16.254": "test-claim-254",
	}
	expectedIP := "192.168.16.250"

	ClaimPool := &IpClaimPool{
		Spec: IpClaimPoolSpec{
			CIDR:      cidr,
			Range:     nil,
			Allocated: allocated,
		},
	}

	checkAvailableIP(ClaimPool, expectedIP, t)

	allocated["192.168.16.250"] = "test-claim-250"
	allocated["192.168.16.251"] = "test-claim-251"
	allocated["192.168.16.253"] = "test-claim-253"

	checkAvailableIP(ClaimPool, "", t)
}
