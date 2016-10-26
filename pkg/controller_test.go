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

package externalip

import (
	"reflect"
	"testing"
	"time"

	"k8s.io/client-go/1.5/pkg/api/v1"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"
)

func TestControllerServicesAdded(t *testing.T) {
	t.Log("started assign ip test")
	source := fcache.NewFakeControllerSource()
	tracker := make(chan string, 6)
	ipHandler := func(iface, cidr string) error {
		t.Logf("received cidr %s for iface %v \n", cidr, iface)
		tracker <- cidr
		return nil
	}
	c := &ExternalIpController{
		Iface:     "eth0",
		Mask:      "24",
		source:    source,
		ipHandler: ipHandler,
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go c.Run(stopCh)

	expectedTracker := make(map[string]int)
	testIps := [][]string{
		{"10.10.0.2", "10.10.0.3"},
		{"10.10.0.2", "10.10.0.3", "10.10.0.4"},
		{"10.10.0.5"},
	}
	for i, ips := range testIps {
		for _, ip := range ips {
			expectedTracker[ip+"/"+c.Mask]++
		}
		source.Add(&v1.Service{
			ObjectMeta: v1.ObjectMeta{Name: "service-" + string(i)},
			Spec:       v1.ServiceSpec{ExternalIPs: ips},
		})
	}

	receivedTracker := make(map[string]int)
	for i := 0; i < 6; i++ {
		select {
		case <-time.After(5 * time.Second):
			t.Errorf("Timed out waiting for processed ips. Currently received: %v", receivedTracker)
		case val := <-tracker:
			receivedTracker[val]++
		}
	}

	if !reflect.DeepEqual(receivedTracker, expectedTracker) {
		t.Errorf("Received: %v are not equal to expected: %v", receivedTracker, expectedTracker)
	}
}

func TestProcessExternalIps(t *testing.T) {
	var rst []string
	ipHandler := func(iface, cidr string) error {
		t.Logf("received cidr %s for iface %v \n", cidr, iface)
		rst = append(rst, cidr)
		return nil
	}
	c := &ExternalIpController{
		Iface:     "eth0",
		Mask:      "24",
		ipHandler: ipHandler,
	}
	testIps := [][]string{
		{"10.10.0.2", "10.10.0.3"},
		{"10.10.0.2", "10.10.0.3", "10.10.0.4"},
		{"10.10.0.5"},
	}

	for _, ips := range testIps {
		rst = []string{}
		expected := []string{}
		for _, ip := range ips {
			expected = append(expected, ip+"/"+c.Mask)
		}
		c.processServiceExternalIPs(&v1.Service{Spec: v1.ServiceSpec{ExternalIPs: ips}})
		if !reflect.DeepEqual(rst, expected) {
			t.Errorf("Expected: %v != Received %v", expected, rst)
		}
	}

}
