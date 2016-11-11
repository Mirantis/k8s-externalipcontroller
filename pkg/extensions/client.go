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
	"k8s.io/client-go/1.5/kubernetes"
	"k8s.io/client-go/1.5/pkg/api"
	"k8s.io/client-go/1.5/pkg/api/unversioned"
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
		clientset,
		&IPNodesGetter{rest},
		&IpClaimGetter{rest},
	}, nil
}

func extensionClient(config *rest.Config) (*rest.RESTClient, error) {
	config.APIPath = "/apis"
	config.ContentConfig = rest.ContentConfig{
		GroupVersion: &unversioned.GroupVersion{
			Group:   GroupName,
			Version: Version,
		},
		NegotiatedSerializer: api.Codecs,
	}
	return rest.RESTClientFor(config)
}

type WrappedClientset struct {
	*kubernetes.Clientset
	*IPNodesGetter
	*IpClaimGetter
}

type IPNodesGetter struct {
	client *rest.RESTClient
}

type IpClaimGetter struct {
	client *rest.RESTClient
}

func (ip *IPNodesGetter) IPNodes() *IPNodesClient {
	return &IPNodesClient{ip.client}
}

type IPNodesClient struct {
	client *rest.RESTClient
}

func (c *IpClaimGetter) IpClaims() *IpClaimClient {
	return &IpClaimClient{c.client}
}

type IpClaimClient struct {
	client *rest.RESTClient
}

func (c *IPNodesClient) Create(ipnode *IpNode) (result *IpNode, err error) {
	ipnode.TypeMeta.Kind = "IpNode"
	result = &IpNode{}
	err = c.client.Post().
		Namespace("default").
		Resource("ipnodes").
		Body(ipnode).
		Do().
		Into(result)
	return
}

func (c *IPNodesClient) List(opts api.ListOptions) (result *IpNodeList, err error) {
	result = &IpNodeList{}
	err = c.client.Get().
		Namespace("default").
		Resource("ipnodes").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
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
	err = c.client.Put().
		Namespace("default").
		Resource("ipnodes").
		Name(ipnode.Name).
		Body(ipnode).
		Do().
		Into(result)
	return
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

func (c *IpClaimClient) List(opts api.ListOptions) (result *IpClaimList, err error) {
	result = &IpClaimList{}
	err = c.client.Get().
		Namespace("default").
		Resource("ipclaims").
		VersionedParams(&opts, api.ParameterCodec).
		Do().
		Into(result)
	return
}

func (c *IpClaimClient) Watch(opts api.ListOptions) (watch.Interface, error) {
	return c.client.Get().
		Namespace("default").
		Prefix("watch").
		Resource("ipclaims").
		VersionedParams(&opts, api.ParameterCodec).
		Watch()
}

func (c *IpClaimClient) Update(ipclaim *IpClaim) (result *IpClaim, err error) {
	result = &IpClaim{}
	err = c.client.Put().
		Namespace("default").
		Resource("ipclaims").
		Name(ipclaim.Name).
		Body(ipclaim).
		Do().
		Into(result)
	return
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
