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
	"net"

	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/meta"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/apimachinery/announced"
	"k8s.io/client-go/1.5/pkg/runtime"

	"github.com/Mirantis/k8s-externalipcontroller/pkg/netutils"
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
		&IpClaimPool{},
		&IpClaimPoolList{},

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

	// used as a heartbeat
	Revision int64 `json:"generation,omitempty"`
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

type IpClaimPool struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard object metadata
	Metadata api.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec IpClaimPoolSpec `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
}

type IpClaimPoolSpec struct {
	CIDR      string            `json:"cidr" protobuf:"bytes,10,opt,name=cidr"`
	Range     []string          `json:"range,omitempty" protobuf:"bytes,5,opt,name=range"`
	Allocated map[string]string `json:"allocated,omitempty" protobuf:"bytes,2,opt,name=allocated"`
}

type IpClaimPoolList struct {
	unversioned.TypeMeta `json:",inline"`

	// Standard list metadata.
	unversioned.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Items []IpClaimPool `json:"items" protobuf:"bytes,2,rep,name=items"`
}

func (p *IpClaimPool) UsableIPs() (ips []string, err error) {
	ip, network, err := net.ParseCIDR(p.CIDR())
	if err != nil {
		return
	}

	//first to be included
	var firstIP net.IP
	//first to be excluded
	var dropOffIP net.IP

	//in case 'Range' is not set for the pool assume ranges of the
	//network itself
	if p.Range() != nil {
		firstIP = net.ParseIP(p.Range()[0])
		dropOffIP = net.ParseIP(p.Range()[len(p.Range())-1])
		netutils.IPIncrement(dropOffIP)
	} else {
		//cast IP to 4 byte representation as netutils.BroadCastIP
		//return data only in such format; the addresses must be in
		//the same format as they are compared in the loop's condition
		firstIP = ip.To4()

		//the network's address and broadcast are not usable
		netutils.IPIncrement(firstIP)
		dropOffIP = netutils.BroadCastIP(network)
	}

	for ip := firstIP; network.Contains(ip) && ip.Equal(dropOffIP) == false; netutils.IPIncrement(ip) {
		ips = append(ips, ip.String())
	}

	return ips, nil
}

func (p *IpClaimPool) AvailableIP() (ip string, err error) {
	usableIPs, err := p.UsableIPs()

	if err != nil {
		return
	}

	for _, ip := range usableIPs {
		if _, exists := p.Allocated()[ip]; !exists {
			return ip, nil
		}
	}

	//TODO(aroma): return custom error in case free IP was not found
	return
}

func (p *IpClaimPool) CIDR() string {
	return p.Spec.CIDR
}

func (p *IpClaimPool) Allocated() map[string]string {
	return p.Spec.Allocated
}

func (p *IpClaimPool) Range() []string {
	return p.Spec.Range
}

func (p *IpClaimPool) GetObjectKind() unversioned.ObjectKind {
	return &p.TypeMeta
}

func (p *IpClaimPool) GetObjectMeta() meta.Object {
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
