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
	"encoding/json"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/meta"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/apimachinery/announced"
	"k8s.io/client-go/1.5/pkg/runtime"
)

const (
	GroupName string = "ipcontroller.ext"
	Version   string = "v1"
)

var (
	SchemeGroupVersion = unversioned.GroupVersion{Group: GroupName, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&IpNode{},
		&IpNodeList{},
		&IpClaim{},
		&IpClaimList{},

		&api.ListOptions{},
		&api.DeleteOptions{},
	)
	return nil
}

func init() {
	if err := announced.NewGroupMetaFactory(
		&announced.GroupMetaFactoryArgs{
			GroupName:                  GroupName,
			VersionPreferenceOrder:     []string{SchemeGroupVersion.Version},
			AddInternalObjectsToScheme: SchemeBuilder.AddToScheme,
		},
		announced.VersionToSchemeFunc{
			SchemeGroupVersion.Version: SchemeBuilder.AddToScheme,
		},
	).Announce().RegisterAndEnable(); err != nil {
		panic(err)
	}
}

type IpNode struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata api.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
}

func (e *IpNode) GetObjectKind() unversioned.ObjectKind {
	return &e.TypeMeta
}

func (e *IpNode) GetObjectMeta() meta.Object {
	return &e.Metadata
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
	Metadata api.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec IpClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (e *IpClaim) GetObjectKind() unversioned.ObjectKind {
	return &e.TypeMeta
}

func (e *IpClaim) GetObjectMeta() meta.Object {
	return &e.Metadata
}

type IpClaimList struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard list metadata.
	unversioned.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpClaim `json:"items" protobuf:"bytes,2,rep,name=items"`
}

type IpClaimSpec struct {
	// NodeName used to identify where IPClaim is assigned (IPNode.Name)
	NodeName string `json:"nodeName" protobuf:"bytes,10,opt,name=nodeName"`
	Cidr     string `json:"cidr,omitempty" protobuf:"bytes,10,opt,name=cidr"`
	Link     string `json:"link" protobuf:"bytes,10,opt,name=link"`
}

// see https://github.com/kubernetes/client-go/issues/8
type ExampleIpNode IpNode
type ExampleIpNodesList IpNodeList
type ExampleIpClaim IpClaim
type ExampleIpClaimList IpClaimList

func (e *IpNode) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpNode{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpNode(tmp)
	*e = tmp2
	return nil
}

func (el *IpNodeList) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpNodesList{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpNodeList(tmp)
	*el = tmp2
	return nil
}

func (e *IpClaim) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpClaim{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpClaim(tmp)
	*e = tmp2
	return nil
}

func (el *IpClaimList) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpClaimList{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpClaimList(tmp)
	*el = tmp2
	return nil
}
