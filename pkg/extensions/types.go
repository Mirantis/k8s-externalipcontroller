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
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/api/v1"
	"k8s.io/client-go/1.5/pkg/runtime"
)

const (
	GroupName string = "ipcontroller.ext"
	Version   string = "v1"
)

var (
	SchemeGroupVersion = unversioned.GroupVersion{Group: GroupName, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme        = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&IpNode{},
		&IpNodeList{},
		&IpClaim{},
		&IpClaimList{},
	)
	return nil
}

func init() {
	if err := AddToScheme(api.Scheme); err != nil {
		panic(err)
	}
}

type IpNode struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard object metadata
	v1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

type IpNodeList struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard list metadata.
	unversioned.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpNode `json:"items" protobuf:"bytes,2,rep,name=items"`
}

type IpClaim struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard object metadata
	v1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec       IpClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	APIVersion string      `json:"apiVersion,omitempty" protobuf:"bytes,3,opt,name=apiVersion"`
}

type IpClaimList struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard list metadata.
	unversioned.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpClaim `json:"items" protobuf:"bytes,2,rep,name=items"`
}

type IpClaimSpec struct {
	// NodeName used to identify where IPClaim is assigned (IPNode.Name)
	NodeName string `json:"nodeName,omitempty" protobuf:"bytes,10,opt,name=nodeName"`
}
