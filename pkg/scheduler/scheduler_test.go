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

	"runtime"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/watch"
	fcache "k8s.io/client-go/1.5/tools/cache/testing"
)

type fakeExtClientset struct {
	mock.Mock
	ipclaims *fakeIpClaims
	ipnodes  *fakeIpNodes
}

func (f *fakeExtClientset) IPClaims() extensions.IPClaimsInterface {
	return f.ipclaims
}

func (f *fakeExtClientset) IPNodes() extensions.IPNodesInterface {
	return f.ipnodes
}

type fakeIpClaims struct {
	mock.Mock
}

func (f *fakeIpClaims) Create(ipclaim *extensions.IpClaim) (*extensions.IpClaim, error) {
	args := f.Called(ipclaim)
	return ipclaim, args.Error(0)
}

func (f *fakeIpClaims) List(_ api.ListOptions) (*extensions.IpClaimList, error) {
	return nil, nil
}

func (f *fakeIpClaims) Update(ipclaim *extensions.IpClaim) (*extensions.IpClaim, error) {
	args := f.Called(ipclaim)
	return args.Get(0).(*extensions.IpClaim), args.Error(1)
}

func (f *fakeIpClaims) Delete(name string, opts *api.DeleteOptions) error {
	args := f.Called(name, opts)
	return args.Error(0)
}

func (f *fakeIpClaims) Watch(_ api.ListOptions) (watch.Interface, error) {
	return nil, nil
}

type fakeIpNodes struct {
	mock.Mock
}

func (f *fakeIpNodes) Create(ipnode *extensions.IpNode) (*extensions.IpNode, error) {
	args := f.Called(ipnode)
	return args.Get(0).(*extensions.IpNode), args.Error(1)
}

func (f *fakeIpNodes) List(_ api.ListOptions) (*extensions.IpNodeList, error) {
	return nil, nil
}

func (f *fakeIpNodes) Update(ipnode *extensions.IpNode) (*extensions.IpNode, error) {
	args := f.Called(ipnode)
	return args.Get(0).(*extensions.IpNode), args.Error(1)
}

func (f *fakeIpNodes) Delete(name string, opts *api.DeleteOptions) error {
	args := f.Called(name, opts)
	return args.Error(0)
}

func (f *fakeIpNodes) Watch(_ api.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func newFakeExtClientset() *fakeExtClientset {
	return &fakeExtClientset{
		ipclaims: &fakeIpClaims{},
		ipnodes:  &fakeIpNodes{},
	}
}

func TestServiceWatcher(t *testing.T) {
	ext := newFakeExtClientset()
	lw := fcache.NewFakeControllerSource()
	stop := make(chan struct{})
	s := ipClaimScheduler{
		DefaultMask:         "24",
		serviceSource:       lw,
		ExtensionsClientset: ext,
	}
	ext.ipclaims.On("Create", mock.Anything).Return(nil)
	go s.serviceWatcher(stop)
	defer close(stop)

	svc := &v1.Service{
		ObjectMeta: v1.ObjectMeta{Name: "test0"},
		Spec:       v1.ServiceSpec{ExternalIPs: []string{"10.10.0.2"}}}
	lw.Add(svc)
	// let controller to process all services
	runtime.Gosched()
	assert.Equal(t, len(ext.ipclaims.Calls), 1, "Unexpected call count to ipclaims")
	createCall := ext.ipclaims.Calls[0]
	ipclaim := createCall.Arguments[0].(*extensions.IpClaim)
	assert.Equal(t, ipclaim.Spec.Cidr, "10.10.0.2/24", "Unexpected cidr assigned to node")
	assert.Equal(t, ipclaim.Name, "10.10.0.2-24", "Unexpected name")

	ext.ipclaims.On("Delete", "10.10.0.2-24", mock.Anything).Return(nil)
	lw.Delete(svc)
	runtime.Gosched()
	assert.Equal(t, len(ext.ipclaims.Calls), 2, "Unexpected call count to ipclaims")
	deleteCall := ext.ipclaims.Calls[1]
	ipclaimName := deleteCall.Arguments[0].(string)
	assert.Equal(t, ipclaimName, "10.10.0.2-24", "Unexpected name")

}
