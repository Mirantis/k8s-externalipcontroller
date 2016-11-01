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

package workqueue

import (
	"fmt"
	"testing"
)

func TestQueue(t *testing.T) {
	queue := NewQueue()
	queue.Add(1)
	queue.Add(2)
	queue.Remove(1)
	item, _ := queue.Get()
	if citem := item.(int); citem != 2 {
		t.Errorf("item expected to be 2 - %v", citem)
	}
	item, _ = queue.Get()
	if item != nil {
		t.Errorf("expected to return nil if empty")
	}
}

func TestProcessQueue(t *testing.T) {
	queue := ProcessingQueue{NewQueue()}
	queue.Add(1)
	queue.Add(2)
	queue.Process(func(item interface{}) error {
		if citem := item.(int); citem != 1 {
			t.Errorf("item expected to be 1 - %v - %v", citem, queue.queue)
		}
		return fmt.Errorf("test")
	})
	queue.Process(func(item interface{}) error {
		if citem := item.(int); citem != 2 {
			t.Errorf("item expected to be 2 - %v - %v", citem, queue.queue)
		}
		return nil
	})
	queue.Process(func(item interface{}) error {
		if citem := item.(int); citem != 1 {
			t.Errorf("item expected to be 1 - %v - %v", citem, queue.queue)
		}
		return nil
	})
}
