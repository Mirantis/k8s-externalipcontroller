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

package scheduler

import (
	"testing"
	"time"

	"runtime"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	fclient "github.com/Mirantis/k8s-externalipcontroller/pkg/extensions/testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"
)

func TestServiceWatcher(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})
	s := ipClaimScheduler{
		DefaultMask:         "24",
		serviceSource:       lw,
		ExtensionsClientset: ext,
	}
	ext.Ipclaims.On("Create", mock.Anything).Return(nil)
	go s.serviceWatcher(stop)
	defer close(stop)

	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "test0"},
		Spec:       v1.ServiceSpec{ExternalIPs: []string{"10.10.0.2"}}}
	lw.Add(svc)
	// let controller to process all services
	runtime.Gosched()
	assert.Equal(t, len(ext.Ipclaims.Calls), 1, "Unexpected call count to ipclaims")
	createCall := ext.Ipclaims.Calls[0]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, ipclaim.Spec.Cidr, "10.10.0.2/24", "Unexpected cidr assigned to node")
	assert.Equal(t, ipclaim.Metadata.Name, "10-10-0-2-24", "Unexpected name")

	ext.Ipclaims.On("Delete", "10-10-0-2-24", mock.Anything).Return(nil)
	lw.Delete(svc)
	runtime.Gosched()
	assert.Equal(t, len(ext.Ipclaims.Calls), 2, "Unexpected call count to ipclaims")
	deleteCall := ext.Ipclaims.Calls[1]
	ipclaimName := deleteCall.Arguments[0].(string)
	assert.Equal(t, ipclaimName, "10-10-0-2-24", "Unexpected name")
}

func TestClaimWatcher(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})
	s := ipClaimScheduler{
		claimSource:         lw,
		ExtensionsClientset: ext,
	}
	go s.claimWatcher(stop)
	defer close(stop)
	claim := &extensions.IpClaim{
		Metadata: api.ObjectMeta{Name: "10.10.0.2-24"},
		Spec:     extensions.IpClaimSpec{Cidr: "10.10.0.2/24"},
	}
	lw.Add(claim)
	ipnodesList := &extensions.IpNodeList{
		Items: []extensions.IpNode{
			{
				Metadata: api.ObjectMeta{Name: "first"},
			},
		},
	}
	ext.Ipnodes.On("List", mock.Anything).Return(ipnodesList, nil)
	ext.Ipclaims.On("Update", mock.Anything).Return(nil)
	runtime.Gosched()
	assert.Equal(t, len(ext.Ipclaims.Calls), 1, "Unexpected calls to ipclaims")
	updatedClaim := ext.Ipclaims.Calls[0].Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, updatedClaim.Metadata.Labels, map[string]string{"ipnode": "first"},
		"Labels should be set to scheduled node")
	assert.Equal(t, updatedClaim.Spec.NodeName, "first", "NodeName should be set to scheduled node")
}

func TestMonitorIpNodes(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	stop := make(chan struct{})
	ticker := make(chan time.Time, 2)
	for i := 0; i < 2; i++ {
		ticker <- time.Time{}
	}
	s := ipClaimScheduler{
		ExtensionsClientset: ext,
		liveIpNodes:         make(map[string]struct{}),
		observedGeneration:  make(map[string]int64),
	}
	ipnodesList := &extensions.IpNodeList{
		Items: []extensions.IpNode{
			{
				Metadata: api.ObjectMeta{
					Name:       "first",
					Generation: 666,
				},
			},
		},
	}
	ipclaimsList := &extensions.IpClaimList{
		Items: []extensions.IpClaim{
			{
				Metadata: api.ObjectMeta{
					Name:   "10.10.0.1-24",
					Labels: map[string]string{"ipnode": "first"},
				},
				Spec: extensions.IpClaimSpec{
					Cidr:     "10.10.0.1/24",
					NodeName: "first",
				},
			},
			{
				Metadata: api.ObjectMeta{
					Name:   "10.10.0.2-24",
					Labels: map[string]string{"ipnode": "first"},
				},
				Spec: extensions.IpClaimSpec{
					Cidr:     "10.10.0.2/24",
					NodeName: "first",
				},
			},
		},
	}
	ext.Ipnodes.On("List", mock.Anything).Return(ipnodesList, nil).Twice()
	ext.Ipclaims.On("List", mock.Anything).Return(ipclaimsList, nil)
	ext.Ipclaims.On("Update", mock.Anything).Return(nil).Twice()
	go s.monitorIPNodes(stop, ticker)
	defer close(stop)
	runtime.Gosched()
	assert.Equal(t, 2, len(ext.Ipnodes.Calls), "Unexpected calls to ipnodes")
	assert.Equal(t, 3, len(ext.Ipclaims.Calls), "Unexpected calls to ipclaims")
	updateCalls := ext.Ipclaims.Calls[1:]
	for _, call := range updateCalls {
		ipclaim := call.Arguments[0].(*extensions.IpClaim)
		assert.Equal(t, ipclaim.Metadata.Labels, map[string]string{}, "monitor should clean all ipclaim labels")
		assert.Equal(t, ipclaim.Spec.NodeName, "", "monitor should clean node name")
	}
	assert.Equal(t, s.isLive("first"), false, "first node shouldn't be considered live")
}
