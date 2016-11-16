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

package claimcontroller

import (
	"runtime"
	"testing"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	fclient "github.com/Mirantis/k8s-externalipcontroller/pkg/extensions/testing"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/errors"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"
)

type fakeIpHandler struct {
	mock.Mock
}

func (f *fakeIpHandler) Add(iface, cidr string) error {
	args := f.Called(iface, cidr)
	return args.Error(0)
}

func (f *fakeIpHandler) Del(iface, cidr string) error {
	args := f.Called(iface, cidr)
	return args.Error(0)
}

func TestClaimWatcher(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	queue := workqueue.NewQueue()
	fiphandler := &fakeIpHandler{}
	defer queue.Close()
	stop := make(chan struct{})
	defer close(stop)
	c := claimController{
		Uid:                 "first",
		Iface:               "eth0",
		ExtensionsClientset: ext,
		claimSource:         lw,
		queue:               queue,
		iphandler:           fiphandler,
	}
	go c.claimWatcher(stop)
	go c.worker()
	claim := &extensions.IpClaim{
		Metadata: api.ObjectMeta{Name: "10.10.0.2-24"},
		Spec:     extensions.IpClaimSpec{Cidr: "10.10.0.2/24", NodeName: "first"},
	}
	fiphandler.On("Add", c.Iface, claim.Spec.Cidr).Return(nil)
	lw.Add(claim)
	runtime.Gosched()
	assert.Equal(t, len(fiphandler.Calls), 1, "Unexpect calls to iphandler")
	assert.Equal(t, fiphandler.Calls[0].Arguments[0].(string), c.Iface, "Unexpected interface passed to netutils")
	assert.Equal(t, fiphandler.Calls[0].Arguments[1].(string), claim.Spec.Cidr, "Unexpected cidr")

	lw.Delete(claim)
	fiphandler.On("Del", c.Iface, claim.Spec.Cidr).Return(nil)
	runtime.Gosched()
	assert.Equal(t, len(fiphandler.Calls), 2, "Unexpect calls to iphandler")
	assert.Equal(t, fiphandler.Calls[1].Arguments[0].(string), c.Iface, "Unexpected interface passed to netutils")
	assert.Equal(t, fiphandler.Calls[1].Arguments[1].(string), claim.Spec.Cidr, "Unexpected cidr")
}

func TestHeartbeatIpNode(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	ticker := make(chan time.Time, 3)
	for i := 0; i < 3; i++ {
		ticker <- time.Time{}
	}
	stop := make(chan struct{})
	c := claimController{
		Uid:                 "first",
		ExtensionsClientset: ext,
	}
	qualResource := unversioned.GroupResource{
		Group:    "ipcontroller",
		Resource: "ipnode",
	}
	ipnode := &extensions.IpNode{
		Metadata: api.ObjectMeta{Name: c.Uid},
	}
	ext.Ipnodes.On("Get", c.Uid).Return(&extensions.IpNode{}, errors.NewNotFound(qualResource, c.Uid))
	ext.Ipnodes.On("Create", mock.Anything).Return(nil)
	ext.Ipnodes.On("Get", c.Uid).Return(ipnode, nil).Twice()
	ext.Ipnodes.On("Update", mock.Anything).Return(nil).Twice()
	go c.heartbeatIpNode(stop, ticker)
	runtime.Gosched()
	assert.Equal(t, len(ext.Ipnodes.Calls), 6, "Expected 6 calls")
}
