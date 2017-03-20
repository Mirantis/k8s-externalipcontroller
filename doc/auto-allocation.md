Auto allocation of external IPs
===============================

Now users can utilize auto allocation feature.
The feature is only available in Claims mode (see ``Basic Modules and Operating
Modes`` document for more detail).

In order to do that, users must first provide
IP pool for the allocator by creation of IpClaimPool resources.
Here is example description for one in yaml format:

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
In the above example, address 192.168.0.251 is not processed by the allocator.

In order to enable auto allocation for particular services, users
must annotate them with following key-value pair - "external-ip = auto"
either on creation or while the service is running (via `kubectl annotate`).
After that, exactly one external IP address will be allocated for the service
and IP claim will be created. Allocated address is then stored in 'allocated'
field of the IP pool object.

Users can create several IP pools. Allocator will process all of them in the
same manner (looking for the first available IP) and with no regard for pools
order, that is users cannot control which pool an address is fetched from.

When service is deleted the allocation is freed and returned to the pool of
usable addresses again. Corresponding IP claim is removed.

Here is a brief demo of the functionality.

[![asciicast](https://asciinema.org/a/6uyrkfn66nufzpuhrwt1veshb.png)](https://asciinema.org/a/6uyrkfn66nufzpuhrwt1veshb)
