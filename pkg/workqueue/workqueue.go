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
	"sync"
)

type QueueAddType interface {
	Add(interface{})
}

type QueueType interface {
	QueueAddType
	Get() (interface{}, bool)
	Done(interface{})
	Remove(interface{})
	Close()
	Len() int
}

func NewQueue() *Queue {
	return &Queue{
		cond:       sync.NewCond(&sync.Mutex{}),
		added:      map[interface{}]bool{},
		processing: map[interface{}]bool{},
		queue:      []interface{}{},
	}
}

type Queue struct {
	cond       *sync.Cond
	added      map[interface{}]bool
	processing map[interface{}]bool
	closed     bool
	queue      []interface{}
}

func (n *Queue) Add(item interface{}) {
	n.cond.L.Lock()
	defer n.cond.L.Unlock()
	if n.closed {
		return
	}
	if _, exists := n.added[item]; exists {
		return
	}
	n.added[item] = true
	if _, exists := n.processing[item]; exists {
		return
	}
	n.queue = append(n.queue, item)
	n.cond.Signal()
}

func (n *Queue) Len() int {
	return len(n.queue)
}

func (n *Queue) Close() {
	n.cond.L.Lock()
	defer n.cond.L.Unlock()
	n.closed = true
	n.cond.Broadcast()
}

func (n *Queue) Get() (item interface{}, quit bool) {
	n.cond.L.Lock()
	defer n.cond.L.Unlock()

	if len(n.queue) == 0 && !n.closed {
		n.cond.Wait()
	}

	if len(n.queue) == 0 {
		return nil, n.closed
	}

	for {
		item, n.queue = n.queue[0], n.queue[1:]
		// item was removed and shouldn't be processed
		if _, exists := n.added[item]; !exists {
			if len(n.queue) == 0 {
				return nil, n.closed
			}
			continue
		}
		break
	}
	n.processing[item] = true
	delete(n.added, item)
	return item, false
}

func (n *Queue) Done(item interface{}) {
	n.cond.L.Lock()
	defer n.cond.L.Unlock()

	delete(n.processing, item)

	if _, exists := n.added[item]; exists {
		n.queue = append(n.queue, item)
		n.cond.Signal()
	}
}

// Remove will prevent item from being processed
func (n *Queue) Remove(item interface{}) {
	n.cond.L.Lock()
	defer n.cond.L.Unlock()

	if _, exists := n.added[item]; exists {
		delete(n.added, item)
	}
}

type ProcessType interface {
	Process(func(item interface{}) error)
}

type ProcessingQueue struct {
	*Queue
}

func (p *ProcessingQueue) Process(f func(item interface{}) error) error {
	item, _ := p.Get()
	defer p.Done(item)
	if err := f(item); err != nil {
		p.Add(item)
		return err
	}
	return nil
}
