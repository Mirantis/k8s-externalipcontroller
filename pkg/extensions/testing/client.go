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

package testing

import (
	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"

	"github.com/stretchr/testify/mock"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/watch"
)

type FakeExtClientset struct {
	mock.Mock
	Ipclaims     *fakeIpClaims
	Ipnodes      *fakeIpNodes
	Ipclaimpools *fakeIpClaimPools
}

func (f *FakeExtClientset) IPClaims() extensions.IPClaimsInterface {
	return f.Ipclaims
}

func (f *FakeExtClientset) IPNodes() extensions.IPNodesInterface {
	return f.Ipnodes
}

func (f *FakeExtClientset) IPClaimPools() extensions.IPClaimPoolsInterface {
	return f.Ipclaimpools
}

type fakeIpClaims struct {
	mock.Mock
}

func (f *fakeIpClaims) Get(name string) (*extensions.IpClaim, error) {
	args := f.Called(name)
	return args.Get(0).(*extensions.IpClaim), args.Error(1)
}

func (f *fakeIpClaims) Create(ipclaim *extensions.IpClaim) (*extensions.IpClaim, error) {
	args := f.Called(ipclaim)
	return ipclaim, args.Error(0)
}

func (f *fakeIpClaims) List(opts api.ListOptions) (*extensions.IpClaimList, error) {
	args := f.Called(opts)
	return args.Get(0).(*extensions.IpClaimList), args.Error(1)
}

func (f *fakeIpClaims) Update(ipclaim *extensions.IpClaim) (*extensions.IpClaim, error) {
	args := f.Called(ipclaim)
	return ipclaim, args.Error(0)
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
	return ipnode, args.Error(0)
}

func (f *fakeIpNodes) List(opts api.ListOptions) (*extensions.IpNodeList, error) {
	args := f.Called(opts)
	return args.Get(0).(*extensions.IpNodeList), args.Error(1)
}

func (f *fakeIpNodes) Update(ipnode *extensions.IpNode) (*extensions.IpNode, error) {
	args := f.Called(ipnode)
	return args.Get(0).(*extensions.IpNode), args.Error(1)
}

func (f *fakeIpNodes) Delete(name string, opts *api.DeleteOptions) error {
	args := f.Called(name, opts)
	return args.Error(0)
}

func (f *fakeIpNodes) Get(name string) (*extensions.IpNode, error) {
	args := f.Called(name)
	return args.Get(0).(*extensions.IpNode), args.Error(1)
}

func (f *fakeIpNodes) Watch(_ api.ListOptions) (watch.Interface, error) {
	return nil, nil
}

type fakeIpClaimPools struct {
	mock.Mock
}

func (f *fakeIpClaimPools) Create(ipclaimpool *extensions.IpClaimPool) (*extensions.IpClaimPool, error) {
	args := f.Called(ipclaimpool)
	return ipclaimpool, args.Error(0)
}

func (f *fakeIpClaimPools) List(opts api.ListOptions) (*extensions.IpClaimPoolList, error) {
	args := f.Called(opts)
	return args.Get(0).(*extensions.IpClaimPoolList), args.Error(1)
}

func (f *fakeIpClaimPools) Update(ipclaimpool *extensions.IpClaimPool) (*extensions.IpClaimPool, error) {
	args := f.Called(ipclaimpool)
	return args.Get(0).(*extensions.IpClaimPool), args.Error(1)
}

func (f *fakeIpClaimPools) Delete(name string, opts *api.DeleteOptions) error {
	args := f.Called(name, opts)
	return args.Error(0)
}

func (f *fakeIpClaimPools) Get(name string) (*extensions.IpClaimPool, error) {
	args := f.Called(name)
	return args.Get(0).(*extensions.IpClaimPool), args.Error(1)
}

func NewFakeExtClientset() *FakeExtClientset {
	return &FakeExtClientset{
		Ipclaims:     &fakeIpClaims{},
		Ipnodes:      &fakeIpNodes{},
		Ipclaimpools: &fakeIpClaimPools{},
	}
}
