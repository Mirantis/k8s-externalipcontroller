Fail-Over Optimization
======================

External IP fail-over functionality depends on both External IP Controller
application capabilities and a number of k8s parameters.
Application itself provides fail-over capabilities in claims mode.
There is a set of k8s parameters that affect pods respawning lag after some
node goes offline. These parameters have different effect on application
fail-over functionality depending on application operating mode.

## Pods Respawning and Corresponding k8s Parameters

In simple mode, it will respawn IP Controller pods so that application is
able to continue operating properly (will respawn corresponding IPs on a new
node).
In claims mode, it will respawn IP Scheduler pods and IP Controller pods. But
in claims mode it is less significant for application as leader election
support for schedulers and monitoring of controllers availability improves
application availability. Application will continue to work properly while at
least one scheduler and one controller remain operational.

## Simple Mode

As there is the only controller part which is running in simple mode, so it is
just kubernetes who cares about controllers' availability. More particularly,
it cares only about pods availability (including pods where controllers are
running). So, when some node goes offline k8s detects that and respawns the
corresponding pods on some another node.

## Claims Mode

Thanks to leader election, scheduler will remain functional while at least one
node has IP Scheduler pods. Scheduler module monitors controllers availability
and reschedules IP claims to healthy controllers from unavailable ones.

Please look at `hb`, `leader-elect` and `monitor` parameters description in
"Basic Modules and Operating Modes" document for details.