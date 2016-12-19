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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	fclient "github.com/Mirantis/k8s-externalipcontroller/pkg/extensions/testing"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/utils"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/1.5/kubernetes/fake"
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

	ext.Ipclaimpools.On("List", mock.Anything).Return(&extensions.IpClaimPoolList{}, nil)

	ext.Ipclaims.On("Create", mock.Anything).Return(nil)
	go s.serviceWatcher(stop)
	defer close(stop)

	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "test0"},
		Spec:       v1.ServiceSpec{ExternalIPs: []string{"10.10.0.2"}}}
	lw.Add(svc)
	// let controller to process all services
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)
	createCall := ext.Ipclaims.Calls[0]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, ipclaim.Spec.Cidr, "10.10.0.2/24", "Unexpected cidr assigned to node")
	assert.Equal(t, ipclaim.Metadata.Name, "10-10-0-2-24", "Unexpected name")

	ext.Ipclaims.On("Delete", "10-10-0-2-24", mock.Anything).Return(nil)
	lw.Delete(svc)
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 2)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)
	deleteCall := ext.Ipclaims.Calls[1]
	ipclaimName := deleteCall.Arguments[0].(string)
	assert.Equal(t, ipclaimName, "10-10-0-2-24", "Unexpected name")
}

func TestAutoAllocationForServices(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})
	svc := v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "need-alloc-svc",
			Annotations: map[string]string{"external-ip": "auto"},
			Namespace:   api.NamespaceDefault,
		},
	}

	fakeClientset := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{svc}})
	s := ipClaimScheduler{
		DefaultMask:         "24",
		serviceSource:       lw,
		ExtensionsClientset: ext,
		Clientset:           fakeClientset,
	}

	poolCIDR := "192.168.16.248/29"
	poolRanges := [][]string{[]string{"192.168.16.250", "192.168.16.252"}}
	pool := &extensions.IpClaimPool{
		Metadata: api.ObjectMeta{Name: "test-pool"},
		Spec: extensions.IpClaimPoolSpec{
			CIDR:   poolCIDR,
			Ranges: poolRanges,
		},
	}
	poolList := &extensions.IpClaimPoolList{Items: []extensions.IpClaimPool{*pool}}

	ext.Ipclaimpools.On("List", mock.Anything).Return(poolList, nil)
	ext.Ipclaimpools.On("Update", mock.Anything).Return(pool, nil)

	ext.Ipclaims.On("Create", mock.Anything).Return(nil)
	ext.Ipclaims.On("Delete", "192-168-16-250-29", mock.Anything).Return(nil)
	ext.Ipclaims.On("Delete", "10-20-0-2-24", mock.Anything).Return(nil)

	go s.serviceWatcher(stop)
	defer close(stop)

	lw.Add(&svc)

	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)

	createCall := ext.Ipclaims.Calls[0]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, "192.168.16.250/29", ipclaim.Spec.Cidr, "Unexpected cidr assigned to node")
	assert.Equal(t, "192-168-16-250-29", ipclaim.Metadata.Name, "Unexpected name")
	assert.Equal(
		t, map[string]string{"ip-pool-name": pool.Metadata.Name},
		ipclaim.Metadata.Labels,
		"Labels was not updated by the pool's CIDR")

	poolUpdateCall := ext.Ipclaimpools.Calls[1]
	p := poolUpdateCall.Arguments[0].(*extensions.IpClaimPool)
	assert.Equal(
		t, p.Spec.Allocated,
		map[string]string{"192.168.16.250": "192-168-16-250-29"},
		"Allocated was not updated correctly for the pool")

	updatedSvc, _ := fakeClientset.Core().Services(svc.ObjectMeta.Namespace).Get(svc.ObjectMeta.Name)
	assert.Equal(t, []string{"192.168.16.250"}, updatedSvc.Spec.ExternalIPs)

	poolList.Items[0].Spec.Allocated = map[string]string{"192.168.16.250": "192-168-16-250-29"}

	changeExternalIPsvc := svc
	changeExternalIPsvc.Spec.ExternalIPs = []string{"10.20.0.2"}
	delete(changeExternalIPsvc.ObjectMeta.Annotations, "external-ip")
	lw.Modify(&changeExternalIPsvc)

	//there should be only 3 calls to Ipclaims at this point:
	//1st - creating of 192-168-16-250-29 in AddFunc
	//2nd - creating of 10-20-0-2-24 in UpdateFunc
	//3rd - deleting of 192-168-16-250-29 in UpdateFunc
	//there must not be additional creation of 192-168-16-250-24 -
	//ip from the pool but with default mask - at the update of the service
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 3)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)

	createCall = ext.Ipclaims.Calls[1]
	ipclaim = createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, "10.20.0.2/24", ipclaim.Spec.Cidr, "Unexpected cidr assigned to node")
	assert.Equal(t, "10-20-0-2-24", ipclaim.Metadata.Name, "Unexpected name")

	deleteCall := ext.Ipclaims.Calls[2]
	claimName := deleteCall.Arguments[0].(string)
	assert.Equal(t, claimName, "192-168-16-250-29", "Unexpected ip claim deleted")

	poolUpdateCall = ext.Ipclaimpools.Calls[4]
	p = poolUpdateCall.Arguments[0].(*extensions.IpClaimPool)
	assert.NotContains(
		t, p.Spec.Allocated,
		"192-168-16-250-29",
		"Allocated should not contain '192-168-16-250-29'")
}

func TestClaimNotCreatedIfExternalIPIsAutoAllocated(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})

	poolCIDR := "192.168.16.248/29"
	poolRanges := [][]string{[]string{"192.168.16.250", "192.168.16.252"}}
	pool := &extensions.IpClaimPool{
		Metadata: api.ObjectMeta{Name: "test-pool"},
		Spec: extensions.IpClaimPoolSpec{
			CIDR:      poolCIDR,
			Ranges:    poolRanges,
			Allocated: map[string]string{"192.168.16.250": "192-168-16-250-29"},
		},
	}
	poolList := &extensions.IpClaimPoolList{Items: []extensions.IpClaimPool{*pool}}

	ext.Ipclaimpools.On("List", mock.Anything).Return(poolList, nil)

	ext.Ipclaims.On("Create", mock.Anything).Return(nil)

	s := ipClaimScheduler{
		DefaultMask:         "24",
		serviceSource:       lw,
		ExtensionsClientset: ext,
	}

	go s.serviceWatcher(stop)
	defer close(stop)

	svc := v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:        "need-alloc-svc",
			Annotations: map[string]string{"external-ip": "auto"},
			Namespace:   api.NamespaceDefault,
		},
		Spec: v1.ServiceSpec{
			ExternalIPs: []string{"192.168.16.250", "172.16.0.2"},
		},
	}
	lw.Add(&svc)
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)

	createCall := ext.Ipclaims.Calls[0]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, "172.16.0.2/24", ipclaim.Spec.Cidr, "Unexpected cidr assigned to node")
	assert.Equal(t, "172-16-0-2-24", ipclaim.Metadata.Name, "Unexpected name")
}

func TestAutoAllocatedOnServiceUpdate(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})

	poolCIDR := "192.168.16.248/29"
	poolRanges := [][]string{[]string{"192.168.16.250", "192.168.16.252"}}
	pool := &extensions.IpClaimPool{
		Metadata: api.ObjectMeta{Name: "test-pool"},
		Spec: extensions.IpClaimPoolSpec{
			CIDR:   poolCIDR,
			Ranges: poolRanges,
		},
	}
	poolList := &extensions.IpClaimPoolList{Items: []extensions.IpClaimPool{*pool}}

	ext.Ipclaimpools.On("List", mock.Anything).Return(poolList, nil)
	ext.Ipclaimpools.On("Update", mock.Anything).Return(pool, nil)

	ext.Ipclaims.On("Create", mock.Anything).Return(nil)

	svc := v1.Service{
		ObjectMeta: v1.ObjectMeta{
			Name:      "need-alloc-svc",
			Namespace: api.NamespaceDefault,
		},
		Spec: v1.ServiceSpec{
			ExternalIPs: []string{"172.16.0.2"},
		},
	}

	fakeClientset := fake.NewSimpleClientset(&v1.ServiceList{Items: []v1.Service{svc}})

	s := ipClaimScheduler{
		DefaultMask:         "24",
		serviceSource:       lw,
		ExtensionsClientset: ext,
		Clientset:           fakeClientset,
	}

	go s.serviceWatcher(stop)
	defer close(stop)

	lw.Add(&svc)

	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)

	annotatedSvc := svc
	annotatedSvc.ObjectMeta.Annotations = map[string]string{"external-ip": "auto"}
	lw.Modify(&annotatedSvc)

	//one extra create for 172-16-0-2-24 as the service is double processed
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 3)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)

	createCall := ext.Ipclaims.Calls[2]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, "192.168.16.250/29", ipclaim.Spec.Cidr, "Unexpected cidr assigned to node")
	assert.Equal(t, "192-168-16-250-29", ipclaim.Metadata.Name, "Unexpected name")

	updatedSvc, _ := fakeClientset.Core().Services(svc.ObjectMeta.Namespace).Get(svc.ObjectMeta.Name)
	assert.Contains(t, updatedSvc.Spec.ExternalIPs, "192.168.16.250")
}

func TestClaimWatcher(t *testing.T) {
	ext := fclient.NewFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})
	s := ipClaimScheduler{
		claimSource:         lw,
		ExtensionsClientset: ext,
		liveIpNodes:         make(map[string]struct{}),
		queue:               workqueue.NewQueue(),
	}
	s.getNode = s.getFairNode
	go s.worker()
	go s.claimWatcher(stop)
	defer close(stop)
	defer s.queue.Close()
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
	s.liveSync.Lock()
	for _, node := range ipnodesList.Items {
		s.liveIpNodes[node.Metadata.Name] = struct{}{}
	}
	s.liveSync.Unlock()
	ext.Ipnodes.On("List", mock.Anything).Return(ipnodesList, nil)
	ext.Ipclaims.On("Update", mock.Anything).Return(nil)
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)
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
		queue:               workqueue.NewQueue(),
	}
	ipnodesList := &extensions.IpNodeList{
		Items: []extensions.IpNode{
			{
				Metadata: api.ObjectMeta{
					Name: "first",
				},
				Revision: 555,
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
	go s.monitorIPNodes(stop, ticker)
	defer close(stop)
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipnodes.Calls), 2)
	}, "Unexpected call count to ipnodes", ext.Ipnodes.Calls)
	utils.EventualCondition(t, time.Second*1, func() bool {
		return assert.ObjectsAreEqual(len(ext.Ipclaims.Calls), 1)
	}, "Unexpected call count to ipclaims", ext.Ipclaims.Calls)
	assert.Equal(t, s.isLive("first"), false, "first node shouldn't be considered live")
}
