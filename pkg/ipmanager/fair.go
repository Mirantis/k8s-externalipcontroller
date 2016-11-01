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

	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	"github.com/coreos/etcd/client"
	"github.com/golang/glog"
)

const (
	defaultEtcdIPCollectionKey = "/ipcontroller/managedips/"
	defaultTTLDuration         = 10 * time.Second
	defaultTTLRenewInterval    = 5 * time.Second
)

var defaultOpts FairEtcdOpts = FairEtcdOpts{defaultEtcdIPCollectionKey, defaultTTLDuration, defaultTTLRenewInterval}

func NewFairEtcd(endpoints []string, stop chan struct{}, queue workqueue.QueueType) (*FairEtcd, error) {
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
	return &FairEtcd{
		client:         kapi,
		stop:           stop,
		ttlInitialized: map[string]bool{},
		opts:           defaultOpts}, nil
}

type FairEtcdOpts struct {
	etcdIPCollectionKey string
	ttlDuration         time.Duration
	ttlRenewInterval    time.Duration
}

type FairEtcd struct {
	client         client.KeysAPI
	stop           chan struct{}
	ttlInitialized map[string]bool
	opts           FairEtcdOpts
}

func (f *FairEtcd) Fit(uid, cidr string) (bool, error) {
	// TODO use watch and cache values
	resp, err := f.client.Get(context.Background(), f.opts.etcdIPCollectionKey, &client.GetOptions{Quorum: true, Recursive: true})
	if err != nil {
		return false, err
	}
	for i := range resp.Node.Nodes {
		if strings.Replace(cidr, "/", "::", -1) == resp.Node.Nodes[i].Key {
			if resp.Node.Nodes[i].Value == uid {
				f.initLeaseManager(cidr, resp.Node.Nodes[i])
				return true, nil
			}
			return false, nil
		}
	}

	if !f.checkFairness(uid, resp.Node.Nodes) {
		return false, nil
	}
	// we need to replace "/" cause etcd will create one more directory because of it
	key := fmt.Sprint(f.opts.etcdIPCollectionKey, strings.Replace(cidr, "/", "::", -1))
	resp, err = f.client.Set(context.Background(), key, uid, &client.SetOptions{TTL: f.opts.ttlDuration, PrevExist: client.PrevNoExist})
	if err != nil {
		return false, err
	}
	f.initLeaseManager(cidr, resp.Node)
	return resp.Node.Value == uid, nil
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

func (f *FairEtcd) ttlRenew(cidr string, node *client.Node) {
	for {
		select {
		case <-f.stop:
			delete(f.ttlInitialized, cidr)
			return
		case <-time.Tick(f.opts.ttlRenewInterval):
			_, err := f.client.Set(
				context.Background(), node.Key, node.Value,
				&client.SetOptions{TTL: f.opts.ttlDuration, PrevValue: node.Value})
			// TODO properly handle error + evict ip from a node
			if err != nil {
				if cerr, ok := err.(client.Error); ok && cerr.Code == client.ErrorCodeTestFailed {
					glog.V(2).Infof("cidr %v acquired by another instace: %v", node.Key, err)
					delete(f.ttlInitialized, cidr)
					// enqueue node.Key for deletion
					return
				}
			}
		}
	}
}

func (f *FairEtcd) initLeaseManager(cidr string, node *client.Node) {
	if f.ttlInitialized == nil {
		return
	}
	if _, ok := f.ttlInitialized[cidr]; !ok {
		go f.ttlRenew(cidr, node)
	}
}

func (f *FairEtcd) loopWatchExpired(stop chan struct{}) {
	watcher := f.client.Watcher(f.opts.etcdIPCollectionKey, &client.WatcherOptions{Recursive: true})
	for {
		select {
		case <-stop:
			return
		default:
			if err := f.watchExpired(watcher); err != nil {
				glog.Errorf("Watcher returned error %v", err)
				continue
			}
		}
	}
}

func (f *FairEtcd) watchExpired(watcher client.Watcher) error {
	resp, err := watcher.Next(context.Background())
	if err != nil {
		glog.Errorf("Watcher erred %v", err)
		return err
	}
	if resp.Action != "expire" {
		return nil
	}
	// enqueue resp.Node.Key for addition
	return nil
}
