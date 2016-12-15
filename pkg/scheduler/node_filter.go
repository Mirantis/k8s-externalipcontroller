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
	"github.com/Mirantis/k8s-externalipcontroller/pkg/extensions"

	"github.com/golang/glog"
)

func (s *ipClaimScheduler) getFairNode(ipnodes []*extensions.IpNode) *extensions.IpNode {
	counter := make(map[string]int)
	for _, key := range s.claimStore.ListKeys() {
		obj, _, err := s.claimStore.GetByKey(key)
		claim := obj.(*extensions.IpClaim)
		if err != nil {
			glog.Errorln(err)
			continue
		}
		if claim.Spec.NodeName == "" {
			continue
		}
		counter[claim.Spec.NodeName]++
	}
	var min *extensions.IpNode
	minCount := -1
	for _, node := range ipnodes {
		count := counter[node.Metadata.Name]
		if minCount == -1 || count < minCount {
			minCount = count
			min = node
		}
	}
	return min
}

func (s *ipClaimScheduler) getFirstNode(ipnodes []*extensions.IpNode) *extensions.IpNode {
	return ipnodes[0]
}
