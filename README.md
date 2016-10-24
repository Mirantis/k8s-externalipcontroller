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

How to run e2e tests?
=====================
You will need working kubernetes cluster. It will be sufficient to use:
```
https://github.com/sttts/kubernetes-dind-cluster
```

Generate k8s config file, it will be required to run e2e tests
```
./cluster/kubectl.sh config view > /tmp/kubeconfig/kubeconfig.yaml
```

Now you can run build + tests scripts (it will be converted to make)

Builds docker image
```
./run_tests.sh -b
```
Export/import image content on a dind node
```
./run_tests.sh -i
```
Run end-to-end tests in container
```
./run_tests.sh -r
```

For convenience you can use all options at once.
