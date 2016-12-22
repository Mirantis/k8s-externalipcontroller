Basic Modules and Operating Modes
=================================

## Basic modules

Application has two basic modules: controller and scheduler. 

Controller module manages IPs on its node (brings up, deletes, ensures that list
of IPs on node reflects the list of IPs required for services).
Controller(s) should be run on nodes that have external connectivity as
External IPs will be spawned on that nodes. Each node should have no more than
one controller at a time.

Scheduler module processes IP claims from services and distributes them among
controllers (i.e. nodes).
Scheduler(s) can be run on any nodes as they just schedule claims among the
controllers.

## Operating Modes

External IP Controller application may be run in one of the operating modes:
* [Simple](#simple-mode)
* [Daemon Set](#daemon-set)

## Simple mode

External IP controller application (its controller module) will be run on one of
the nodes. It should be run on node that has external connectivity as all the
External IPs will be spawned on that node.
Kubernetes provides fail-over for External IP controller application. So, when
there's a problem with k8s node, External IP controller will be spawned on
another k8s worker node and will bring External IPs up on that node.
Simple mode is easy to setup and takes less resources. It makes sense when all
IPs should be brought up on the same node. However, fail-over in this mode takes
longer than in daemon set mode (k8s detects node failure in much longer
intervals by default, see node-monitor-grace-period, pod-eviction-timeout; these
could be minimized but there was no research on how safe is it to set them to
say 2-5 seconds).

## Daemon Set mode

External IP controller application will be run on several nodes. One or more
controller modules and one or more scheduler modules will be run. Controller
modules should be run on nodes where IPs are expected to be spawned. Scheduler
modules can be run on any nodes. There is no much sense to run more than one
scheduler on every particular node. Several scheduler modules are run to provide
HA for scheduler (A/B mode). So, that in case of no response from active
scheduler (e.g. on node failure) another one becomes active. If more than one
scheduler will be used then scheduler election mode should be switched on
(parameter `leader-elect=true`).
There can be different rules of IPs distribution among controllers (i.e. nodes).
It is controlled by `nodefilter` parameter. Default rule is to distribute IPs
evenly among all controllers (`nodefilter=fair`). Alternative one is
`nodefilter=first-alive` when all IPs will be spawned on the first available
controller (i.e. node).


