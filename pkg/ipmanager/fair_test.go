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
	"testing"

	"github.com/coreos/etcd/client"
	"golang.org/x/net/context"
)

type testKeysApi struct {
	collection map[string]*client.Node
}

type testWatcher struct{}

func (w *testWatcher) Next(ctx context.Context) (*client.Response, error) {
	return nil, nil
}

func (k *testKeysApi) Get(ctx context.Context, key string, opts *client.GetOptions) (*client.Response, error) {
	if opts.Recursive {
		var nodes client.Nodes
		for _, node := range k.collection {
			nodes = append(nodes, node)
		}
		return &client.Response{Node: &client.Node{Nodes: nodes}}, nil
	}
	return &client.Response{Node: k.collection[key]}, nil
}

func (k *testKeysApi) Set(ctx context.Context, key, value string, opts *client.SetOptions) (*client.Response, error) {
	var modifyIndex int
	if _, ok := k.collection[key]; !ok {
		modifyIndex = 0
	} else {
		modifyIndex = k.collection[key].ModifiedIndex + 1
	}
	k.collection[key] = &client.Node{Value: value, Key: key, ModifiedIndex: modifyIndex}
	return &client.Response{Node: k.collection[key]}, nil
}

func (k *testKeysApi) Delete(ctx context.Context, key string, opts *client.DeleteOptions) (*client.Response, error) {
	return nil, nil
}

func (k *testKeysApi) Create(ctx context.Context, key, value string) (*client.Response, error) {
	return nil, nil
}

func (k *testKeysApi) CreateInOrder(ctx context.Context, dir, value string, opts *client.CreateInOrderOptions) (*client.Response, error) {
	return nil, nil
}

func (k *testKeysApi) Update(ctx context.Context, key, value string) (*client.Response, error) {
	return nil, nil
}

func (k *testKeysApi) Watcher(key string, opts *client.WatcherOptions) client.Watcher {
	return &testWatcher{}
}

func failIfErr(t *testing.T, err error) {
	if err != nil {
		t.Errorf("Unexcpeted error %v", err)
	}
}

func TestFairManager(t *testing.T) {
	client := &testKeysApi{collection: map[string]*client.Node{}}
	stop := make(chan struct{})
	fair := &FairEtcd{client: client, stop: stop}
	fit, err := fair.Fit("1", "10.10.0.2/24")
	failIfErr(t, err)
	if !fit {
		t.Errorf("Expected to fit on Uid 1 %v", client.collection)
	}
	fit, err = fair.Fit("2", "10.10.0.3/24")
	failIfErr(t, err)
	if !fit {
		t.Errorf("Expected to fit on Uid 2 %v", client.collection)
	}
	fit, err = fair.Fit("2", "10.10.0.4/24")
	failIfErr(t, err)
	if !fit {
		t.Errorf("Expected to fit on Uid 2 %v", client.collection)
	}
	fit, err = fair.Fit("2", "10.10.0.5/24")
	failIfErr(t, err)
	if fit {
		t.Errorf("Expected not to fit on Uid 2 %v", client.collection)
	}
	fit, err = fair.Fit("1", "10.10.0.5/24")
	failIfErr(t, err)
	if !fit {
		t.Errorf("Expected not to fit on Uid 1 %v", client.collection)
	}
	close(stop)
}
