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

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/apis/extensions/v1beta1"
)

func EnsureThirdPartyResourcesExist(clientset *kubernetes.Clientset) error {
	resourceNames := []string{"ip-node", "ip-claim", "ip-claim-pool"}
	for _, resName := range resourceNames {
		if err := ensureThirdPartyResource(clientset, resName); err != nil {
			return err
		}
	}

	return nil
}

func ensureThirdPartyResource(clientset *kubernetes.Clientset, name string) error {
	fullName := strings.Join([]string{name, GroupName}, ".")
	_, err := clientset.Extensions().ThirdPartyResources().Get(fullName)
	if err == nil {
		return nil
	}

	resource := &v1beta1.ThirdPartyResource{
		Versions: []v1beta1.APIVersion{
			{Name: Version},
		}}
	resource.SetName(fullName)
	_, err = clientset.Extensions().ThirdPartyResources().Create(resource)
	return err
}
