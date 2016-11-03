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

package integration

import (
	"github.com/Mirantis/k8s-externalipcontroller/pkg/ipmanager"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Etcd", func() {

	It("Verifies that fair ip manager works correctly with etcd", func() {
		stop := make(chan struct{})
		queue := workqueue.NewQueue()
		defer close(stop)
		defer queue.Close()
		fair, err := ipmanager.NewFairEtcd([]string{"http://localhost:4001"}, stop, queue)
		Expect(err).NotTo(HaveOccurred())
		fit, err := fair.Fit("1", "10.10.0.2/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).To(BeTrue())
		fit, err = fair.Fit("2", "10.10.0.2/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).NotTo(BeTrue())
	})
})
