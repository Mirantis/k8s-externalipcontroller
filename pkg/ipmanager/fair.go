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
	"strings"
	"time"

	"golang.org/x/net/context"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	"github.com/coreos/etcd/client"
	"github.com/golang/glog"
)

const (
	defaultEtcdIPCollectionKey = "/ipcontroller/managedips/"
	defaultTTLDuration         = 4 * time.Second
	defaultTTLRenewInterval    = 2 * time.Second
)

var defaultOpts FairEtcdOpts = FairEtcdOpts{defaultEtcdIPCollectionKey, defaultTTLDuration, defaultTTLRenewInterval}

// NewFairEtcd will do 3 things:
// 1. Validate that is it fair for certain node to acquire IP
// 2. Watch expired IPs and try to acquire them
// 3. Renew ttl for acquired IPs, remove it from a node in case it was acquired by another node
func NewFairEtcd(endpoints []string, stop chan struct{}, queue workqueue.QueueType) (*FairEtcd, error) {
	glog.V(0).Infof("EXPIREMENTAL")
	cfg := client.Config{
		Endpoints:               endpoints,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}
	c, err := client.New(cfg)
	if err != nil {
		return nil, err
	}
	kapi := client.NewKeysAPI(c)
	f := &FairEtcd{
		client:         kapi,
		stop:           stop,
		ttlInitialized: map[string]bool{},
		opts:           defaultOpts,
		queue:          queue}
	go f.loopWatchExpired(stop)
	return f, nil
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
	queue          workqueue.QueueType
}

func (f *FairEtcd) Fit(uid, cidr string) (bool, error) {
	glog.V(10).Infof("Trying to fit %s on %s", cidr, uid)

	// REMOVE THIS ASAP
	f.client.Set(context.Background(), keyFromCidr(f.opts.etcdIPCollectionKey, uid), uid, &client.SetOptions{
		TTL: f.opts.ttlDuration,
	})

	resp, err := f.client.Get(context.Background(), f.opts.etcdIPCollectionKey, &client.GetOptions{
		Quorum:    true,
		Recursive: true})
	if err != nil {
		return false, err
	}
	for i := range resp.Node.Nodes {
		if cidr == cidrFromKey(resp.Node.Nodes[i].Key) {
			if resp.Node.Nodes[i].Value == uid {
				f.initLeaseManager(cidr, resp.Node.Nodes[i])
				return true, nil
			}
			return false, nil
		}
	}

	if !f.checkFairness(uid, resp.Node.Nodes) {
		glog.V(10).Infof("Not fair to acquire %s on %s", uid, cidr)
		return false, nil
	}
	// we need to replace "/" cause etcd will create one more directory because of it
	key := keyFromCidr(f.opts.etcdIPCollectionKey, cidr)
	resp, err = f.client.Set(context.Background(), key, uid, &client.SetOptions{
		TTL:       f.opts.ttlDuration,
		PrevExist: client.PrevNoExist})
	glog.V(10).Infof("Tried to acquire %s on %s. Response %v", cidr, uid, resp)
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
	glog.V(2).Infof("Starting ttl renew for cidr %s on node %v", cidr, node.Value)
	defer delete(f.ttlInitialized, cidr)
	for {
		select {
		case <-f.stop:
			return
		case <-time.Tick(f.opts.ttlRenewInterval):
			_, err := f.client.Set(
				context.Background(), node.Key, node.Value,
				&client.SetOptions{
					TTL:       f.opts.ttlDuration,
					PrevValue: node.Value})
			glog.V(4).Infof("Renewing ttl for key %v value %v", node.Key, node.Value)
			if err != nil {
				if cerr, ok := err.(client.Error); ok && cerr.Code == client.ErrorCodeTestFailed {
					glog.V(2).Infof("cidr %v acquired by another instance: %v", node.Key, err)
					f.queue.Add(&netutils.DelCIDR{cidr})
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
			glog.V(2).Infof("Exiting watcher loop")
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
	glog.V(2).Infof("Received expire response %v", resp)
	f.queue.Add(&netutils.AddCIDR{cidrFromKey(resp.Node.Key)})
	return nil
}

func (f *FairEtcd) CleanIPCollection() error {
	_, err := f.client.Delete(context.Background(), f.opts.etcdIPCollectionKey, &client.DeleteOptions{
		Dir:       true,
		Recursive: true,
	})
	return err
}

func cidrFromKey(key string) string {
	// /collections/10.10.0.2::24
	cidrParts := strings.Split(key, "/")
	cidr := strings.Split(cidrParts[len(cidrParts)-1], "::")
	return strings.Join(cidr, "/")
}

func keyFromCidr(prefix, cidr string) string {
	return strings.Join([]string{prefix, strings.Replace(cidr, "/", "::", -1)}, "")
}
