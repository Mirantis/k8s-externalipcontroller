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
	"strings"
	"testing"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	"github.com/stretchr/testify/mock"

	"k8s.io/client-go/1.5/pkg/api/v1"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"
)

type fakeIpHandler struct {
	mock.Mock
	syncer chan struct{}
}

func (f *fakeIpHandler) Add(iface, cidr string) error {
	args := f.Called(iface, cidr)
	f.syncer <- struct{}{}
	return args.Error(0)
}

func (f *fakeIpHandler) Del(iface, cidr string) error {
	args := f.Called(iface, cidr)
	f.syncer <- struct{}{}
	return args.Error(0)
}

func TestControllerServicesAddwed(t *testing.T) {
	t.Log("started assign ip test")
	source := fcache.NewFakeControllerSource()
	syncer := make(chan struct{}, 6)
	fake := &fakeIpHandler{syncer: syncer}
	c := &ExternalIpController{
		Iface:     "eth0",
		Mask:      "24",
		source:    source,
		ipHandler: fake,
		queue:     workqueue.NewQueue(),
	}

	stopCh := make(chan struct{})
	defer close(stopCh)
	go c.Run(stopCh)

	testIps := [][]string{
		{"10.10.0.2", "10.10.0.3"},
		{"10.10.0.2", "10.10.0.3", "10.10.0.4"},
		{"10.10.0.5"},
	}

	for i, ips := range testIps {
		for _, ip := range ips {
			fake.On("Add", c.Iface, strings.Join([]string{ip, c.Mask}, "/")).Return(nil)
		}
		source.Add(&v1.Service{
			ObjectMeta: v1.ObjectMeta{Name: "service-" + string(i)},
			Spec:       v1.ServiceSpec{ExternalIPs: ips},
		})
	}

	for i := 0; i < 6; i++ {
		select {
		case <-time.After(200 * time.Millisecond):
			t.Errorf("Waiting for calls failed. Current calls %v", fake.Calls)
		case <-fake.syncer:
		}
	}
}

func TestProcessExternalIps(t *testing.T) {
	fake := &fakeIpHandler{syncer: make(chan struct{}, 6)}
	c := &ExternalIpController{
		Iface:     "eth0",
		Mask:      "24",
		ipHandler: fake,
		queue:     workqueue.NewQueue(),
	}
	testIps := [][]string{
		{"10.10.0.2", "10.10.0.3"},
		{"10.10.0.2", "10.10.0.3", "10.10.0.4"},
		{"10.10.0.5"},
	}
	go c.worker()

	for _, ips := range testIps {
		for _, ip := range ips {
			fake.On("Add", c.Iface, strings.Join([]string{ip, c.Mask}, "/")).Return(nil)
		}
		c.processServiceExternalIPs(&v1.Service{Spec: v1.ServiceSpec{ExternalIPs: ips}})
	}

	for i := 0; i < 6; i++ {
		select {
		case <-time.After(200 * time.Millisecond):
			t.Errorf("Waiting for calls failed. Current calls %v", fake.Calls)
		case <-fake.syncer:
		}
	}
}
