Fail-Over Optimization
======================

External IP fail-over functionality depends on both External IP Controller
application capabilities and a number of k8s parameters.
Application itself provides fail-over capabilities in claims mode.
There is a set of k8s parameters that affect pods respawning lag after some
node goes offline. These parameters have different effect on application
fail-over functionality depending on application operating mode.

## Pods Respawning and Corresponding k8s Parameters

Kubernetes cares only about pods availability including pods where External IP
Controller application is running. So, when some node goes offline k8s detects
that and respawns the corresponding pods on some another node.

The following parameters affect pods respawning lag:
* `--node-status-update-frequency` - kubelet's parameter, 10s by default;
* `--node-monitor-period` - parameter of kube-controller-manager, 5s by default;
* `--node-monitor-grace-period` - parameter of kube-controller-manager, 40s by
default;
* `--pod-eviction-timeout` - parameter of kube-controller-manager, 5m by default;
Please refer to kubernetes docs for more detail. In short, `node-monitor-grace-period`
determines how fast node will be considered as being offline,
`node-status-update-frequency` and `node-monitor-period` should be proportionally
smaller than that. `pod-eviction-timeout` determines when pods will be
respawned on some other node after failed node status will be changed to `NotReady`.
Changing default values to `--node-status-update-frequency=4s`,
`--node-monitor-period=2s`, `--node-monitor-grace-period=16s`,
`--pod-eviction-timeout=20s` allows to decrease pods respawning lag from 5 minutes
to about 20 seconds. These values were tested on a lab with 5-6 nodes clusters.

## Simple Mode

In simple mode, kubernetes will respawn IP Controller pods so that application
is able to continue operating properly (will respawn corresponding IPs on a new
node).
As there is the only controller part which is running in simple mode, so it is
just kubernetes who cares about controllers' availability.

## Claims Mode

In claims mode, kubernetes will respawn IP Scheduler pods and IP Controller
pods. But in claims mode it is less significant for application as leader
election support for schedulers and monitoring of controllers reachability
improves application availability.
Thanks to leader election, scheduler will remain functional while at least one
node has IP Scheduler pods. Scheduler module monitors controllers availability
and reschedules IP claims to healthy controllers from unavailable ones.
In overall, application will continue to work properly while at least one
scheduler and one controller remain operational.
There is a set of application parameters that affect IP fail-over: 
`hb`, `leader-elect` and `monitor`. For parameters description, please refer to
"Basic Modules and Operating Modes" document for more detail.

## Known Issues

Regardless of the settings described above there are chances that pods from 
failed nodes will be respawned with about 15-20 minutes lag. This behaviour was
observed with k8s v.1.5.x and calico v1.1.0-rc3, v1.1.0-rc5 within 25-30% cases.
Two situations have been observed that were leading to such behaviour:
* kube-proxy does not resetup routes so applications cannot access kube-API when
some of the master nodes go down;
* pods are not respawned for a long time after some of the worker nodes go down.
