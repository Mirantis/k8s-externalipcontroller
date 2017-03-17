Application Deployment Examples
===============================

The application can be run in one of the following operating modes:
* Simple mode
* Claims mode

You can get more information about operating modes in the ``Basic Modules and
Operating Modes`` document.

Let's look at how to deploy the application in a k8s cluster.

We assume that:
* Public network (network for External IPs) is routable to nodes.
* We want to expose services to external clients via external IPs.

There is a set of deployment configuration examples in the `examples` directory
that we will use here.

The following is the description of CLI command flow that is used to deploy
and operate the application. However, it is the same flow that is in use for
any other k8s application.

In order to run the application, one needs to use `kubectl apply` providing
the appropriate configuration file(s):
* Simple mode: `kubectl apply -f examples/simple/externalipcontroller.yaml`
* Claims mode: `kubectl apply -f examples/claims/controller.yaml -f examples/claims/scheduler.yaml`

To check running pods, run `kubectl get pods -o wide`. Application pods should
appear in the output::

    NAME                              READY     STATUS    RESTARTS   AGE       IP              NODE
    claimcontroller-5k4lx             1/1       Running   0          54m       10.250.1.13     k8s-03
    claimcontroller-95xhc             1/1       Running   0          54m       10.250.1.12     k8s-02
    claimcontroller-l5rhr             1/1       Running   0          54m       10.250.1.11     k8s-01
    claimscheduler-1170328893-5m2nb   1/1       Running   0          54m       10.233.75.195   k8s-05
    claimscheduler-1170328893-pwmhd   1/1       Running   0          54m       10.233.126.2    k8s-03

To check third party resources (TPR), that the application uses, run
`kubectl get thirdpartyresources`. If everything went OK, the following should
be in the list::

    NAME                             DESCRIPTION   VERSION(S)
    ip-claim-pool.ipcontroller.ext                 v1
    ip-claim.ipcontroller.ext                      v1
    ip-node.ipcontroller.ext                       v1

Each of these resources can be got/altered separately.
E.g., `kubectl get ipnode` should return all the nodes where the controller
modules are running.

There is an nginx deployment example in `examples/nginx.yaml` that can be used
to test External IPs functioning. One can alter `externalIPs` section according
to environment settings and run nginx server using `kubectl apply -f` command.
Given set of IPs should be set up by the External IP Controller then. It can be
checked with `curl` and by retrieving `ipclaim` TPR::

    $ kubectl get ipclaim --show-labels
    NAME          KIND                          LABELS
    10-0-0-7-24   IpClaim.v1.ipcontroller.ext   ipnode=k8s-01
    10-0-0-8-24   IpClaim.v1.ipcontroller.ext   ipnode=k8s-02

For the information regarding `ipclaimpool` resource, please refer to the ``Auto
allocation of external IPs`` document.

To remove the application, please delete corresponding daemon set and deployment
using `kubectl delete` command.
