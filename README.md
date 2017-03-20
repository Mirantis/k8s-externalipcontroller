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
