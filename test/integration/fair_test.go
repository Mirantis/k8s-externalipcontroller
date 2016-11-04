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
	"fmt"
	"time"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/ipmanager"
	"github.com/Mirantis/k8s-externalipcontroller/pkg/workqueue"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Fair [etcd]", func() {

	It("Verifies that fair ip manager works correctly with etcd", func() {
		stop1 := make(chan struct{})
		stop2 := make(chan struct{})
		queue1 := workqueue.NewQueue()
		queue2 := workqueue.NewQueue()
		defer queue1.Close()
		defer queue2.Close()
		fair1, err := ipmanager.NewFairEtcd([]string{"http://localhost:4001"}, stop1, queue1)
		fair2, err := ipmanager.NewFairEtcd([]string{"http://localhost:4001"}, stop2, queue2)
		defer fair1.CleanIPCollection()
		Expect(err).NotTo(HaveOccurred())

		By("validating that fair manager evenly distributes ips")
		fit, err := fair1.Fit("1", "10.10.0.2/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).To(BeTrue())
		fit, err = fair2.Fit("2", "10.10.0.2/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).NotTo(BeTrue())
		fit, err = fair1.Fit("1", "10.10.0.3/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).NotTo(BeTrue())
		fit, err = fair2.Fit("2", "10.10.0.3/24")
		Expect(err).NotTo(HaveOccurred())
		Expect(fit).To(BeTrue())
		if lth := queue1.Len(); lth != 0 {
			Fail("Queue length %d != 0", lth)
		}
		close(stop1)
		Eventually(func() error {
			fit, err := fair2.Fit("2", "10.10.0.2/24")
			if err != nil {
				return err
			}
			if !fit {
				return fmt.Errorf("Expected to acquire current cidr after it will expire")
			}
			return nil
		}, 10*time.Second, 1*time.Second).Should(BeNil())
	})
})
