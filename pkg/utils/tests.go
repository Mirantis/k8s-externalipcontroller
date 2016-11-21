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

package utils

import (
	"time"

	"github.com/stretchr/testify/assert"
)

func EventualCondition(t assert.TestingT, wait time.Duration, comp assert.Comparison, msgAndArgs ...interface{}) bool {
	for {
		select {
		case <-time.After(wait):
			return assert.Condition(t, comp, msgAndArgs...)
		case <-time.Tick(10 * time.Millisecond):
			if comp() {
				return true
			}
		}
	}
}
