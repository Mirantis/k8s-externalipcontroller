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

package extensions

import (
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func EnsureThirdPartyResourcesExist(ki kubernetes.Interface) error {
	resourceNames := []string{"ip-node", "ip-claim", "ip-claim-pool"}
	for _, resName := range resourceNames {
		if err := ensureThirdPartyResource(ki, resName); err != nil {
			return err
		}
	}

	return nil
}

func RemoveThirdPartyResources(ki kubernetes.Interface) {
	for _, name := range []string{"ip-node", "ip-claim", "ip-claim-pool"} {
		fullName := strings.Join([]string{name, GroupName}, ".")
		ki.Extensions().ThirdPartyResources().Delete(fullName, &metav1.DeleteOptions{})
	}
}

func ensureThirdPartyResource(ki kubernetes.Interface, name string) error {
	fullName := strings.Join([]string{name, GroupName}, ".")
	_, err := ki.Extensions().ThirdPartyResources().Get(fullName, metav1.GetOptions{})
	if err == nil {
		return nil
	}

	resource := &v1beta1.ThirdPartyResource{
		Versions: []v1beta1.APIVersion{
			{Name: Version},
		}}
	resource.SetName(fullName)
	_, err = ki.Extensions().ThirdPartyResources().Create(resource)
	return err
}

func WaitThirdPartyResources(ext ExtensionsClientset, timeout time.Duration, interval time.Duration) (err error) {
	timeoutChan := time.After(timeout)
	intervalChan := time.Tick(interval)
	for {
		select {
		case <-timeoutChan:
			return err
		case <-intervalChan:
			_, err = ext.IPClaims().List(metav1.ListOptions{})
			if err != nil {
				continue
			}
			_, err = ext.IPNodes().List(metav1.ListOptions{})
			if err != nil {
				continue
			}
			_, err = ext.IPClaimPools().List(metav1.ListOptions{})
			if err != nil {
				continue
			}
			return nil
		}
	}
}
