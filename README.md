External ip controller [![Build Status](https://travis-ci.org/Mirantis/k8s-externalipcontroller.svg?branch=master)](https://travis-ci.org/Mirantis/k8s-externalipcontroller)
======================

[![asciicast](https://asciinema.org/a/95449.png)](https://asciinema.org/a/95449)

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
