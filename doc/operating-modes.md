Basic Modules and Operating Modes
=================================

## Basic modules

Application consists of two basic modules: controller and scheduler. 

Controller module manages IPs on its node (brings up, deletes, ensures that list
of IPs on node reflects the list of IPs required for services).
Controller(s) should be run on nodes that have external connectivity as
External IPs will be spawned on those nodes. Each node should have no more than
one controller at a time.

Scheduler module processes IP claims from services and distributes them among
the controllers (i.e. nodes).
Scheduler(s) can be run on any nodes as they just schedule claims among the
controllers. Scheduler module is not obligatory to be run. It is not in use
while application runs in Simple mode.

## Operating Modes

External IP Controller application may be run in one of the operating modes:
* [Simple](#simple-mode)
* [Claims](#claims-mode)

## Simple mode

External IP controller application (its controller module) will be run on one of
the nodes. It should be run on node that has external connectivity as all the
External IPs will be spawned on that node.
Kubernetes provides fail-over for External IP controller application. So, when
there's a problem with k8s node, External IP controller will be spawned on
another k8s worker node and will bring External IPs up on that node.
Simple mode is easy to setup and takes less resources. It makes sense when all
IPs should be brought up on the same node. However, fail-over in this mode takes
longer than in Claims mode (k8s detects node failure in much longer intervals by
default, see `node-monitor-grace-period`, `pod-eviction-timeout`;
these could be minimized but there was no research on how safe is it to set them
to some 2-5 seconds) and it may work wrong in some cases.

# Parameters

Next command-line parameters are available in Simple mode for controller module:
* `iface` - interface that will be used to assign IP addresses (default "eth0").
* `kubeconfig` - kubeconfig to use with kubernetes client (default "").
* `mask` - mask part of network CIDR (default "32").
* `resync` - interval to resync state for all ips (default 20 sec).
It is usually enough to set `iface` and `mask` parameters.

## Claims mode

External IP controller application will be run on several nodes. One or more
controller modules and one or more scheduler modules will be run. Controller
modules should be run on nodes where IPs are expected to be spawned. Scheduler
modules can be run on any nodes. There is no much sense to run more than one
scheduler on every particular node. Several scheduler modules are run to provide
HA for scheduler (A/B mode). So, that in case of no response from active
scheduler (e.g. on node failure) another one becomes active. If more than one
scheduler will be used then scheduler election mode should be switched on
(parameter `leader-elect=true`) otherwise there can be race condition between
schedulers.
It is better not to run scheduler on same node as controller because in case of
node outage IP fail-over will take more time.

# IPs distribution in Claims mode

There can be different rules of IPs distribution among controllers (i.e. nodes)
in Claims mode. This is controlled by `nodefilter` parameter. Default rule
is to distribute IPs evenly among all the controllers (`nodefilter=fair`).
Alternative rule is `nodefilter=first-alive` where all IPs will be spawned on
the first available controller (i.e. node). Claims mode with `first-alive`
rule is similar to Simple mode but with more responsive and correct fail-over.

# Parameters

Next command-line parameters are available in Claims mode for controller module:
* `iface` - interface that will be used to assign IP addresses (default "eth0").
* `hb` - how often to send heartbeats from controllers (default 2 sec).
* `kubeconfig` - kubeconfig to use with kubernetes client (default "").
* `mask` - mask part of network CIDR (default "32").
* `resync` - interval to resync state for all ips (default 20 sec).
* `hostname` - use provided hostname instead of os.Hostname (default
os.Hostname).

Next command-line parameters are available in Claims mode for scheduler module:
* `kubeconfig` - kubeconfig to use with kubernetes client (default "").
* `mask` - mask part of network CIDR (default "32").
*	`nodefilter` - node filter to use while dispatching IP claims; it controls IPs
distribution between controllers (default "fair").
* `monitor`, 4*time.Second, how often to check controllers liveness (default 4
sec).

