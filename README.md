External ip controller [![Build Status](https://travis-ci.org/Mirantis/k8s-externalipcontroller.svg?branch=master)](https://travis-ci.org/Mirantis/k8s-externalipcontroller)
======================

[![asciicast](https://asciinema.org/a/95449.png)](https://asciinema.org/a/95449)

Auto allocation of external IPs
===============================

Now users can utilize auto allocation feature.

In order to do that they must first provide
IP pool for the allocator by creation of IpClaimPool resources.
Here is example description for one in yaml format

```yaml
apiVersion: ipcontroller.ext/v1
kind: IpClaimPool
metadata:
    name: test-pool
spec:
    cidr: 192.168.0.248/29
    ranges:
        - - 192.168.0.249
          - 192.168.0.250
        - - 192.168.0.252
          - 192.168.0.253
```

Few words about the Spec: CIDR is mandatory to supply.
Ranges can be omitted (range of the whole network defined in "CIDR" field is used then).
Exclusion of particular addresses is done via specifying multiple ranges.
In example above address 192.168.0.251 is not processed by the allocator.

In order to enable auto allocation for particular services users
must annotate them with following key-value pair - "external-ip = auto"
either on creation or while the service is running (via `kubectl annotate`).
After that there will be allocated exactly one external IP address for the service
and IP claim will be created. Allocated address is then stored in 'allocated' field of pool
object.

Users can create several IP pools. Allocator will process all of them in the same manner
(looking for first available IP) and with no regard for order, that is users cannot control
which pool an address is fetched from.

When service is deleted allocation is freed and returned to the pool of usable addresses
again. Corresponding IP claim is removed.

Here is a brief demo of the functionality.

[![asciicast](https://asciinema.org/a/6uyrkfn66nufzpuhrwt1veshb.png)](https://asciinema.org/a/6uyrkfn66nufzpuhrwt1veshb)

How to run tests?
================

Install dependencies and prepare kube dind cluster
```
make get-deps
```

Build necessary images and run tests
```
make test
```
