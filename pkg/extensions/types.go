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
	"errors"
	"net"

	"k8s.io/apimachinery/pkg/apimachinery/announced"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/pkg/api"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
)

const (
	GroupName string = "ipcontroller.ext"
	Version   string = "v1"
)

var (
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}
	SchemeBuilder      = runtime.NewSchemeBuilder(addKnownTypes)
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(
		SchemeGroupVersion,
		&IpNode{},
		&IpNodeList{},
		&IpClaim{},
		&IpClaimList{},
		&IpClaimPool{},
		&IpClaimPoolList{},

		&metav1.GetOptions{},
		&metav1.ListOptions{},
		&metav1.DeleteOptions{},
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
	).Announce(api.GroupFactoryRegistry).RegisterAndEnable(api.Registry, api.Scheme); err != nil {
		panic(err)
	}
}

type IpNode struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// used as a heartbeat
	Revision int64 `json:",string"`
}

func (e *IpNode) GetObjectKind() schema.ObjectKind {
	return &e.TypeMeta
}

func (e *IpNode) GetObjectMeta() metav1.Object {
	return &e.Metadata
}

type IpNodeList struct {
	metav1.TypeMeta `json:",inline"`

	// Standard list metadata.
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpNode `json:"items" protobuf:"bytes,2,rep,name=items"`
}

type IpClaim struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec IpClaimSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

func (e *IpClaim) GetObjectKind() schema.ObjectKind {
	return &e.TypeMeta
}

func (e *IpClaim) GetObjectMeta() metav1.Object {
	return &e.Metadata
}

type IpClaimList struct {
	metav1.TypeMeta `json:",inline"`

	// Standard list metadata.
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpClaim `json:"items" protobuf:"bytes,2,rep,name=items"`
}

type IpClaimSpec struct {
	// NodeName used to identify where IPClaim is assigned (IPNode.Name)
	NodeName string `json:"nodeName" protobuf:"bytes,10,opt,name=nodeName"`
	Cidr     string `json:"cidr,omitempty" protobuf:"bytes,10,opt,name=cidr"`
	Link     string `json:"link" protobuf:"bytes,10,opt,name=link"`
}

type IpClaimPool struct {
	metav1.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec IpClaimPoolSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

type IpClaimPoolSpec struct {
	CIDR      string            `json:"cidr" protobuf:"bytes,10,opt,name=cidr"`
	Ranges    [][]string        `json:"ranges,omitempty" protobuf:"bytes,5,opt,name=ranges"`
	Allocated map[string]string `json:"allocated,omitempty" protobuf:"bytes,2,opt,name=allocated"`
}

type IpClaimPoolList struct {
	metav1.TypeMeta `json:",inline"`

	// Standard list metadata.
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpClaimPool `json:"items" protobuf:"bytes,2,rep,name=items"`
}

func (p *IpClaimPool) AvailableIP() (availableIP string, err error) {
	ip, network, err := net.ParseCIDR(p.Spec.CIDR)
	if err != nil {
		return
	}

	var dropOffIP net.IP

	ranges := [][]net.IP{}

	//in case 'Ranges' is not set for the pool assume ranges of the
	//network itself
	if p.Spec.Ranges != nil {
		for _, r := range p.Spec.Ranges {
			ip = net.ParseIP(r[0])
			dropOffIP = net.ParseIP(r[len(r)-1])
			netutils.IPIncrement(dropOffIP)

			ranges = append(ranges, []net.IP{ip, dropOffIP})
		}
	} else {
		//network address is not usable
		netutils.IPIncrement(ip)
		ranges = [][]net.IP{{ip, dropOffIP}}
	}

	for _, r := range ranges {
		curAddr := r[0]
		nextAddr := make(net.IP, len(curAddr))
		copy(nextAddr, curAddr)
		netutils.IPIncrement(nextAddr)

		firstOut := r[len(r)-1]

		for network.Contains(curAddr) && network.Contains(nextAddr) && !curAddr.Equal(firstOut) {
			if _, exists := p.Spec.Allocated[curAddr.String()]; !exists {
				return curAddr.String(), nil
			}
			netutils.IPIncrement(curAddr)
			netutils.IPIncrement(nextAddr)

		}
	}

	return "", errors.New("There is no free IP left in the pool")
}

func (p *IpClaimPool) GetObjectKind() schema.ObjectKind {
	return &p.TypeMeta
}

func (p *IpClaimPool) GetObjectMeta() metav1.Object {
	return &p.Metadata
}

// see https://github.com/kubernetes/client-go/issues/8
type ExampleIpNode IpNode
type ExampleIpNodesList IpNodeList
type ExampleIpClaim IpClaim
type ExampleIpClaimList IpClaimList
type ExampleIpClaimPool IpClaimPool
type ExampleIpClaimPoolList IpClaimPoolList

func (e *IpClaimPool) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpClaimPool{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpClaimPool(tmp)
	*e = tmp2
	return nil
}

func (e *IpClaimPoolList) UnmarshalJSON(data []byte) error {
	tmp := ExampleIpClaimPoolList{}
	err := json.Unmarshal(data, &tmp)
	if err != nil {
		return err
	}
	tmp2 := IpClaimPoolList(tmp)
	*e = tmp2
	return nil
}

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
