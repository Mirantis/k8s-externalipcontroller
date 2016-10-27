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

package ipmanager

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/client"
)

const (
	etcdIPCollectionKey = "/ipcontroller/managedips/"
)

func NewFairEtcd(endpoints []string, stop chan struct{}) (*FairEtcd, error) {
	cfg := client.Config{
		Endpoints: endpoints,
		Transport: client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	kapi := client.NewKeysAPI(c)
	return &FairEtcd{client: kapi, stop: stop}, nil
}

type FairEtcd struct {
	client client.KeysAPI
	stop   chan struct{}
}

func (f *FairEtcd) Fit(uid, cidr string) (bool, error) {
	// TODO use watch and cache values
	resp, err := f.client.Get(context.Background(), etcdIPCollectionKey, &client.GetOptions{Quorum: true, Recursive: true})
	if err != nil {
		return false, err
	}
	if !f.checkOwnership(uid, cidr, resp.Node.Nodes) {
		return false, nil
	}

	if !f.checkFairness(uid, resp.Node.Nodes) {
		return false, nil
	}
	// we need to replace "/" cause etcd will create one more directory because of it
	key := fmt.Sprint(etcdIPCollectionKey, strings.Replace(cidr, "/", "::", -1))
	resp, err = f.client.Set(context.Background(), key, uid, &client.SetOptions{TTL: time.Second * 10, PrevExist: client.PrevNoExist})
	if err != nil {
		return false, err
	}
	go f.ttlRenew(resp.Node)
	return resp.Node.Value == uid, nil
}

// checkOwnership verifies that given cidr is not used by someone else
func (f *FairEtcd) checkOwnership(uid, cidr string, nodes client.Nodes) bool {
	for _, node := range nodes {
		if node.Key == cidr {
			return uid == node.Value
		}
	}
	return true
}

// checkFairness verifies that client with provided uid is underutilized
func (f *FairEtcd) checkFairness(uid string, nodes client.Nodes) bool {
	counter := make(map[string]int)
	min := true
	for _, node := range nodes {
		counter[node.Value]++
	}
	for node := range counter {
		if node == uid {
			continue
		}
		if counter[uid] > counter[node] {
			min = false
		}
	}
	return min
}

func (f *FairEtcd) ttlRenew(node *client.Node) {
	index := node.ModifiedIndex
	for {
		select {
		case <-c.stop:
			return
		case <-time.Tick(5 * time.Second).c:
			resp, err := f.client.Set(
				context.Background(), node.Key, node.Value,
				&client.SetOptions{TTL: time.Second * 10, PrevIndex: index})
			// TODO properly handle error + evict ip from a node
			if err != nil {
				glog.Errorf("was acquired by someone else")
				return
			}
			index = resp.Node.ModifiedIndex
		}
	}
}
