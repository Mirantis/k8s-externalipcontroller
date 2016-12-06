ECMP Load Balancing for External IPs
====================================

- [ECMP Load Balancing for External IPs](#ecmp-load-balancing-for-external-ips)
  * [Introduction](#introduction)
  * [Proposed solution](#proposed-solution)
  * [Deployment scheme](#deployment-scheme)
    + [Diagram](#diagram)
    + [Description](#description)
  * [Single rack network topology](#single-rack-network-topology)
    + [Diagram](#diagram-1)
    + [Description](#description-1)
    + [Example](#example)
  * [Multirack network topology](#multirack-network-topology)
    + [Diagram](#diagram-2)
    + [Description](#description-2)
    + [Example](#example-1)

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

This document describes how to expose Kubernetes services using  External IP
controller and how to provide load balancing and high availability for External
IPs based on Equal-cost multi-path routing (ECMP). Proposed network topologies
should be considered as examples.

## Deployment scheme

### Diagram

![Deployment scheme](images/ecmp-scheme.png)

### Description

* External IP controller kubernetes application is running on all nodes (or on
a subet of nodes) as a DeamonSet or ReplicaSet with node affinity (up to cloud
operator).
* On start every External IP controller instance pulls information about
services from `kube-api` and brings up all External IPs on `lo` interface on
all target nodes.
* Every External IP controller instance watches `kube-api` for updates in
services with External IPs and:
    * When new External IPs appear every instance brings them up on `lo`.
    * When service is removed every instance removes appropriate External IPs
    from the interface.
* External IP BGP speaker k8s application is running on the same nodes as
External IP controller. It monitors local system and announces External IPs
found on `lo` via BGP.

## Single rack network topology

### Diagram

![Simple network topology](images/bgp-for-ecmp-single.png)

### Description

* There's one instance of Route-Reflector application running in the AS. This
application (k8s PoD) should have static IP which should migrate with it to
the new node in case of fail-over. Otherwise Top of Rack router configuration
should be updated when Route-Reflector IP changes.
* ExIP BGP speakers and Route-Reflector are configured as neighbors within
the same AS.
* Route-Reflector is peered with Top of Rack router within the same AS.
* External IPs assigned by External IP controller on `lo` are exported by
ExIP BGP speakers to the Route-Reflector and thus to Top of Rack router.
* Top of Rack router should have ECMP enabled/configured (for virtual lab and
BIRD routing daemon `merge paths` should be enabled in `kernel` protocol to
support ECMP in routing table).

### Example

Let's see how it works on example:
* Kuberenetes cluster is running on 3 nodes:
```
node1 10.210.1.11
node2 10.210.1.12
node3 10.210.1.13
```
* External IP controller is running on 2 nodes (node1 and node3),
[example](../examples/simple/externalipcontroller.yaml) with `replicas: 2` and
`HOST_INTERFACE` env variable set to `lo`.
* User creates a service with external IPs ([example](../examples/nginx.yaml)):
```
  externalIPs:
    - 10.0.0.7
    - 10.0.0.8
```
* External IP controller assigns those IPs to `lo` interface of node1 and
node3:
```
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet 10.0.0.8/32 scope global lo
       valid_lft forever preferred_lft forever
    inet 10.0.0.7/32 scope global lo
       valid_lft forever preferred_lft forever
```
* ExIP BGP speakers (containers with BIRD) detect changes on node1/node3 and
announce them to peers via BGP. Example of BIRD export filter:
```
if ( ifname = "lo" ) then {
  if net != 127.0.0.0/8 then accept;
}
```
* Top of Rack router (BIRD daemon with `merge paths` enabled) configures the
following routes:
```
10.0.0.7  proto bird
        nexthop via 10.210.1.11  dev eth2 weight 1
        nexthop via 10.210.1.13  dev eth2 weight 1
10.0.0.8  proto bird
        nexthop via 10.210.1.11  dev eth2 weight 1
        nexthop via 10.210.1.13  dev eth2 weight 1
```

## Multirack network topology

### Diagram

![Multirack network topology](images/bgp-for-ecmp.png)

### Description

* There's one instance of Route-Reflector application running per rack/AS. This
application (k8s PoD) should have static IP which should migrate with it to
the new node in case of fail-over. Otherwise Top of Rack router configuration
should be updated when Route-Reflector IP changes.
* ExIP BGP speakers and Route-Reflector are configured as neighbors within the
same AS.
See [ExIP BGP speaker configuration example](examples/bird-node1.cfg).
* Route-Reflector is peered with Top of Rack router within the same AS. ECMP
should be configured on both sides (for virtual lab and BIRD routing daemon
`add paths` should be enabled in BGP protocol).
See [Route-Reflector configuration example](examples/bird-rr1.cfg).
* External IPs assigned by External IP controller on `lo` are exported by
ExIP BGP speakers to the Route-Reflector and thus to Top of Rack router.
* Top of Rack router should have ECMP enabled/configured (for virtual lab and
BIRD routing daemon `merge paths` should be enabled in `kernel` protocol to
support ECMP in routing table).
See [Top of Rack router BGP configuration example](examples/bird-tor1.cfg).
* Core router is peered with Top of Rack routers via eBGP. It also has ECMP
enabled/configured (for virtual lab and BIRD routing daemon `merge paths`
should be enabled in `kernel` protocol to support ECMP in routing table).
See [Core router BGP configuration example](examples/bird-core.cfg).

### Example

![Multirack example topology](images/bgp-for-ecmp-example.png)

* `nginx` pod and service are running ([example](../examples/nginx.yaml)):
```
default   nginx-3086523004-0aqgr   1/1   Running   0    1h    10.233.80.3    node-01-003
default   nginx-3086523004-3t6hq   1/1   Running   0    2m    10.233.79.67   node-02-002
default   nginx-3086523004-ca67h   1/1   Running   0    1h    10.233.85.4    node-01-002
```
* Let's check route from `node-01-002` to nginx POD running in rack2:
```
$ traceroute -n 10.233.79.67
traceroute to 10.233.79.67 (10.233.79.67), 30 hops max, 60 byte packets
 1  10.211.1.254  0.352 ms  0.316 ms  0.247 ms
 2  192.168.192.5  0.301 ms  0.409 ms  0.353 ms
 3  192.168.192.10  0.459 ms  0.392 ms  0.703 ms
 4  10.211.2.2  0.933 ms  0.879 ms  1.166 ms
 5  10.233.79.67  1.453 ms  1.396 ms  1.246 ms
```
* `nginx` service has 2 external IPs (10.0.0.7 and 10.0.0.8). External IP controller
assigned those IPs to `lo` interface on all nodes:
```
1: lo: <LOOPBACK,UP,LOWER_UP> mtu 65536 qdisc noqueue state UNKNOWN group default qlen 1
    inet 127.0.0.1/8 scope host lo
       valid_lft forever preferred_lft forever
    inet 10.0.0.8/32 scope global lo
       valid_lft forever preferred_lft forever
    inet 10.0.0.7/32 scope global lo
       valid_lft forever preferred_lft forever
```
* Routing tables on the routers:

Core router
```
10.0.0.7  proto bird
        nexthop via 192.168.192.6  dev rack01a weight 1
        nexthop via 192.168.192.10  dev rack02a weight 1
10.0.0.8  proto bird
        nexthop via 192.168.192.6  dev rack01a weight 1
        nexthop via 192.168.192.10  dev rack02a weight 1
```

TOR1
```
10.0.0.7  proto bird
        nexthop via 10.211.1.2  dev eth2 weight 1
        nexthop via 10.211.1.3  dev eth2 weight 1
10.0.0.8  proto bird
        nexthop via 10.211.1.2  dev eth2 weight 1
        nexthop via 10.211.1.3  dev eth2 weight 1
```

TOR2 (no multipath since we have only one node in rack2)
```
10.0.0.7 via 10.211.2.2 dev eth3  proto bird
10.0.0.8 via 10.211.2.2 dev eth3  proto bird
```
