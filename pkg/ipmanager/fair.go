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
	"flag"
	"fmt"
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/coreos/etcd/client"
)

var ENDPOINTS string

func init() {
	flag.StringVar(&ENDPOINTS, "etcdendpoints", "http://localhost:4001", "endpoint for etcd client")
}

func NewFairEtcd() (*FairEtcd, error) {
	cfg := client.Config{
		Endpoints: []string{ENDPOINTS},
		Transport: client.DefaultTransport,
		// set timeout per request to fail fast when the target endpoint is unavailable
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	kapi := client.NewKeysAPI(c)
	return &FairEtcd{client: kapi}, nil
}

type FairEtcd struct {
	client client.KeysAPI
}

func (f *FairEtcd) Fit(uid, cidr string) (bool, error) {
	// TODO change it with watch and cache
	resp, err := f.client.Get(context.Background(), "/ipcontroller/externalips/", &client.GetOptions{Quorum: true, Recursive: true})
	if err != nil {
		return false, err
	}
	if !f.countFairness(uid, resp.Node.Nodes) {
		return false, nil
	}
	// we need to replace "/" cause etcd will create one more child because of it
	key := fmt.Sprint("/ipcontroller/externalips/", strings.Replace(cidr, "/", "::", -1))
	resp, err = f.client.Set(context.Background(), key, uid, &client.SetOptions{TTL: time.Second * 30, PrevExist: client.PrevNoExist})
	if err != nil {
		return false, err
	}
	return resp.Node.Value == uid, nil
}

func (f *FairEtcd) countFairness(uid string, nodes client.Nodes) bool {
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
