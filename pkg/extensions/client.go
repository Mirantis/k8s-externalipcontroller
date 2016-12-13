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
	"bytes"
	"encoding/json"

	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
	"k8s.io/client-go/1.5/pkg/runtime"
	"k8s.io/client-go/1.5/pkg/runtime/serializer"
	"k8s.io/client-go/1.5/pkg/watch"
	"k8s.io/client-go/1.5/rest"
)

func WrapClientsetWithExtensions(clientset *kubernetes.Clientset, config *rest.Config) (*WrappedClientset, error) {
	restConfig := &rest.Config{}
	*restConfig = *config
	rest, err := extensionClient(restConfig)
	if err != nil {
		return nil, err
	}
	return &WrappedClientset{
		Client: rest,
	}, nil
}

func extensionClient(config *rest.Config) (*rest.RESTClient, error) {
	config.APIPath = "/apis"
	config.ContentConfig = rest.ContentConfig{
		GroupVersion: &unversioned.GroupVersion{
			Group:   GroupName,
			Version: Version,
		},
		NegotiatedSerializer: serializer.DirectCodecFactory{CodecFactory: api.Codecs},
		ContentType:          runtime.ContentTypeJSON,
	}
	return rest.RESTClientFor(config)
}

type ExtensionsClientset interface {
	IPNodes() IPNodesInterface
	IPClaims() IPClaimsInterface
	IPClaimPools() IPClaimPoolsInterface
}

type WrappedClientset struct {
	Client *rest.RESTClient
}

type IPClaimsInterface interface {
	Create(*IpClaim) (*IpClaim, error)
	Get(name string) (*IpClaim, error)
	List(api.ListOptions) (*IpClaimList, error)
	Watch(api.ListOptions) (watch.Interface, error)
	Update(*IpClaim) (*IpClaim, error)
	Delete(string, *api.DeleteOptions) error
}

type IPNodesInterface interface {
	Create(*IpNode) (*IpNode, error)
	Get(name string) (*IpNode, error)
	List(api.ListOptions) (*IpNodeList, error)
	Watch(api.ListOptions) (watch.Interface, error)
	Update(*IpNode) (*IpNode, error)
	Delete(string, *api.DeleteOptions) error
}

type IPClaimPoolsInterface interface {
	Create(*IpClaimPool) (*IpClaimPool, error)
	Get(name string) (*IpClaimPool, error)
	List(api.ListOptions) (*IpClaimPoolList, error)
	Update(*IpClaimPool) (*IpClaimPool, error)
	Delete(string, *api.DeleteOptions) error
}

func (w *WrappedClientset) IPNodes() IPNodesInterface {
	return &IPNodesClient{w.Client}
}

func (w *WrappedClientset) IPClaims() IPClaimsInterface {
	return &IpClaimClient{w.Client}
}

func (w *WrappedClientset) IPClaimPools() IPClaimPoolsInterface {
	return &IpClaimPoolClient{w.Client}
}

type IPNodesClient struct {
	client *rest.RESTClient
}

type IpClaimClient struct {
	client *rest.RESTClient
}

type IpClaimPoolClient struct {
	client *rest.RESTClient
}

func decodeResponseInto(resp []byte, obj interface{}) error {
	return json.NewDecoder(bytes.NewReader(resp)).Decode(obj)
}

func (c *IPNodesClient) Create(ipnode *IpNode) (result *IpNode, err error) {
	result = &IpNode{}
	resp, err := c.client.Post().
		Namespace("default").
		Resource("ipnodes").
		Body(ipnode).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IPNodesClient) List(opts api.ListOptions) (result *IpNodeList, err error) {
	result = &IpNodeList{}
	resp, err := c.client.Get().
		Namespace("default").
		Resource("ipnodes").
		LabelsSelectorParam(opts.LabelSelector).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IPNodesClient) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Namespace("default").
		Prefix("watch").
		Resource("ipnodes").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

func (c *IPNodesClient) Update(ipnode *IpNode) (result *IpNode, err error) {
	result = &IpNode{}
	resp, err := c.client.Put().
		Namespace("default").
		Resource("ipnodes").
		Name(ipnode.Metadata.Name).
		Body(ipnode).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IPNodesClient) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Namespace("default").
		Resource("ipnodes").
		Name(name).
		Body(options).
		Do().
		Error()
}

func (c *IPNodesClient) Get(name string) (result *IpNode, err error) {
	result = &IpNode{}
	resp, err := c.client.Get().
		Namespace("default").
		Resource("ipnodes").
		Name(name).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimClient) Get(name string) (result *IpClaim, err error) {
	result = &IpClaim{}
	err = c.client.Get().
		Namespace("default").
		Resource("ipclaims").
		Name(name).
		Do().
		Into(result)

	return result, err
}

func (c *IpClaimClient) Create(ipclaim *IpClaim) (result *IpClaim, err error) {
	result = &IpClaim{}
	resp, err := c.client.Post().
		Namespace("default").
		Resource("ipclaims").
		Body(ipclaim).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimClient) List(opts api.ListOptions) (result *IpClaimList, err error) {
	result = &IpClaimList{}
	resp, err := c.client.Get().
		Namespace("default").
		Resource("ipclaims").
		LabelsSelectorParam(opts.LabelSelector).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimClient) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Namespace("default").
		Prefix("watch").
		Resource("ipclaims").
		Param("resourceVersion", opts.ResourceVersion).
		Watch()
}

func (c *IpClaimClient) Update(ipclaim *IpClaim) (result *IpClaim, err error) {
	result = &IpClaim{}
	resp, err := c.client.Put().
		Namespace("default").
		Resource("ipclaims").
		Name(ipclaim.Metadata.Name).
		Body(ipclaim).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimClient) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Namespace("default").
		Resource("ipclaims").
		Name(name).
		Body(options).
		Do().
		Error()
}

func (c *IpClaimPoolClient) Get(name string) (result *IpClaimPool, err error) {
	result = &IpClaimPool{}
	err = c.client.Get().
		Namespace("default").
		Resource("ipclaimpools").
		Name(name).
		Do().
		Into(result)

	return result, err
}

func (c *IpClaimPoolClient) Create(ipclaimpool *IpClaimPool) (result *IpClaimPool, err error) {
	result = &IpClaimPool{}
	resp, err := c.client.Post().
		Namespace("default").
		Resource("ipclaimpools").
		Body(ipclaimpool).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimPoolClient) List(opts api.ListOptions) (result *IpClaimPoolList, err error) {
	result = &IpClaimPoolList{}
	resp, err := c.client.Get().
		Namespace("default").
		Resource("ipclaimpools").
		VersionedParams(&opts, api.ParameterCodec).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}

func (c *IpClaimPoolClient) Delete(name string, options *api.DeleteOptions) error {
	return c.client.Delete().
		Namespace("default").
		Resource("ipclaimpools").
		Name(name).
		Body(options).
		Do().
		Error()
}

func (c *IpClaimPoolClient) Update(ipclaimpool *IpClaimPool) (result *IpClaimPool, err error) {
	result = &IpClaimPool{}
	resp, err := c.client.Put().
		Namespace("default").
		Resource("ipclaimpools").
		Name(ipclaimpool.Metadata.Name).
		Body(ipclaimpool).
		DoRaw()
	if err != nil {
		return result, err
	}
	return result, decodeResponseInto(resp, result)
}
