External ip controller
======================

Download
```
github.com/Mirantis/k8s-externalipcontroller
```

Install glide
```
curl https://glide.sh/get | sh
```

Download dependencies
```
glide install --strip-vendor
```

How to run unit and integration tests?
======================================
Unit tests:
```
go test ./pkg/
```

To run integration tests you need sudo (network namespaces created for tests)
```
go test -o _output/integration.test -c ./test/integration/
sudo ./_output/integration.test
```

How to run e2e tests?
=====================
You will need working kubernetes cluster. It will be sufficient to use:
```
https://github.com/sttts/kubernetes-dind-cluster
```

Now you can run build + tests scripts (it will be converted to make)

Builds docker image
```
./run_tests.sh -b
```
Export/import image content to a dind node
```
./run_tests.sh -i
```
Run end-to-end tests in container
```
./run_tests.sh -r
```

For convenience you can use all options at once (in right order):
```
./run_tests.sh -bir
```
