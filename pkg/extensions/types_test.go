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

func checkFreeIP(p *IpClaimPool, expected string, t *testing.T) {
	actual, err := p.AvailableIP()
	if err != nil {
		t.Errorf("Error must not occur during AvailableIP() method; details --> %v\n", err)
	}

	if expected != actual {
		t.Errorf("Actual free IP %v is not as expected %v\n", actual, expected)
	}
}

func TestIpClaimPoolAvailableIPFromCIDR(t *testing.T) {
	cidr := "192.168.16.248/29"
	allocated := map[string]string{
		"192.168.16.249": "test-claim-249",
		"192.168.16.250": "test-claim-250",
		"192.168.16.252": "test-claim-252",
		"192.168.16.253": "test-claim-253",
	}
	expectedIP := "192.168.16.251"

	ClaimPool := &IpClaimPool{
		Spec: IpClaimPoolSpec{
			CIDR:      cidr,
			Range:     nil,
			Allocated: allocated,
		},
	}

	checkFreeIP(ClaimPool, expectedIP, t)

	allocated["192.168.16.254"] = "test-claim-254"
	allocated["192.168.16.251"] = "test-claim-251"

	checkFreeIP(ClaimPool, "", t)
}

func TestIpClaimPoolAvailableIPFromRange(t *testing.T) {
	cidr := "192.168.16.248/29"
	IPrange := []string{"192.168.16.250", "192.168.16.252"}

	allocated := map[string]string{
		"192.168.16.250": "test-claim-250",
		"192.168.16.251": "test-claim-251",
	}

	expectedIP := "192.168.16.252"

	ClaimPool := &IpClaimPool{
		Spec: IpClaimPoolSpec{
			CIDR:      cidr,
			Range:     IPrange,
			Allocated: allocated,
		},
	}

	checkFreeIP(ClaimPool, expectedIP, t)

	allocated["192.168.16.252"] = "test-claim-252"

	checkFreeIP(ClaimPool, "", t)
}
