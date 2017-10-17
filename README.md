External IP Controller [![Build Status](https://travis-ci.org/Mirantis/k8s-externalipcontroller.svg?branch=master)](https://travis-ci.org/Mirantis/k8s-externalipcontroller)
======================

## Introduction

One of the possible ways to expose k8s services on a bare metal deployments is
using External IPs. Each node runs a kube-proxy process which programs `iptables`
rules to trap requests to External IPs and redirect them to the correct backends.

So, in order to access k8s service from the outside, we just need to route public
traffic to one of the k8s worker nodes which have `kube-proxy` running and thus
have all the needed `iptables` rules for External IPs configured.

## Proposed solution

External IP Controller is a k8s application which is deployed on top of k8s
cluster and which configures External IPs on k8s worker node(s) to provide
IP connectivity.

## Demo

[![asciicast](https://asciinema.org/a/95449.png)](https://asciinema.org/a/95449)

How to run tests
================

Install dependencies and prepare kubernetes dind cluster. It is supposed that
Go v.1.7.x has been installed already.
```
make get-deps
```

Build necessary images and run tests.
```
make test
```

Use ```make help``` to see all the options available.

How to start using this?
========================

Both controller and scheduller operate on third party resources and require them 
to be created. Since kubernetes 1.7 most of the installations enable RBAC.
For this reason we need to grant our application correct permissions. For
testing envrionment you can use:
```
kubectl apply -f examples/auth.yaml
```

In case you are using kubeadm dind environment - deploy claim controller and scheduller like this: 
```
kubectl apply -f examples/claims/
```
For any other environment you need to ensure that `--iface` option in 
examples/claims/controller.yaml file is correct. This interface will be used for IP assignment.

If you want to use auto allocation from IP pool - you need to create atleast one such pool.
We provided an example in file `examples/ip-pool.yml`. It can be applied with kubectl after
third party resources will be created.
We are not resyncing services after pool was created, so please ensure that it is created
before you will start requesting IPs.

We also have one basic example with nginx service and pods - `examples/nginx.yaml`. This example
creates deployment for nginx with single replica and service of type LoadBalancer.

For each service that require ip we will create ipclaim object. You can list all ipclaims with:
```
kubectl get ipclaims
```

Notes on CI and end-to-end tests
================================
In tests we want to verify that IPs are reachable remotely. For this purpose we are using --testlink option in e2e tests. 
During the tests we will configure that link with IP from a network that is used in tests. 
This is also the reason why we are running e2e tests with sudo.
The requirement here is that all kubernetes nodes must be in the same L2 domain.
In our application we are assigning IPs to a node. In dind-based setup those nodes are regular containers.
Therefore to guarantee connectivity in our CI we need to assign IP on a bridge used by docker.

For simplicity we want to limit number of running ipcontrollers to 2. To make it work with kubeadm-dind-cluster 
we have to set label ipcontroller= on kube workers.  And in the test we are using this label as node selector for daemonset pods.
