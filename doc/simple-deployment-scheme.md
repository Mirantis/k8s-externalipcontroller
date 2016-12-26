Simple Deployment Scheme
========================

## Introduction

One of the possible ways to expose k8s services on a bare metal deployments is
using External IPs. Each node runs a kube-proxy process which programs `iptables`
rules to trap access to External IPs and redirect them to the correct backends.

So in order to access k8s service from the outside we just need to route public
traffic to one of k8s worker nodes which has `kube-proxy` running and thus has
needed `iptables` rules for External IPs.

## Proposed solution

External IP controller is k8s application which is deployed on top of k8s
cluster and which configures External IPs on k8s worker node(s) to provide
IP connectivity.

This document describes a simple deployment scheme for External IP controller.

We assume that:
* Public network (network for External IPs) is routable to nodes.
* We want to expose services to external clients via external IPs.

## Deployment scheme

![Deployment scheme](images/simple-scheme.png)

Description:
* External IP controller kubernetes application is running on one of the nodes
(`replicas=1`).
* On start it pulls information about services from `kube-api` and brings up
all External IPs on the specified interface (eth0 in our example above).
* It watches `kube-api` for updates in services with External IPs and:
    * When new External IPs appear it brings them up.
    * When service is removed it removes appropriate External IPs from the
    interface.
* Kubernetes provides fail-over for External IP controller. Since we have
`replicas` set to 1, then we'll have only one instance running in a cluster to
avoid IPs duplication. And when there's a problem with k8s node, External IP
controller will be spawned on a new k8s worker node and bring External IPs up
on that node.
