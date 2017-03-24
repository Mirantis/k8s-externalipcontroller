# Copyright 2017 Mirantis
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


IMAGE_REPO ?= mirantis/k8s-externalipcontroller
IMAGE_TAG ?= latest
DOCKER_BUILD ?= no

BUILD_DIR = _output
VENDOR_DIR = vendor
ROOT_DIR = $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

# kubeadm-dind-cluster supports k8s versions:
# "v1.4", "v1.5" and "v1.6".
DIND_CLUSTER_VERSION ?= v1.4

ENV_PREPARE_MARKER = .env-prepare.complete
BUILD_IMAGE_MARKER = .build-image.complete


ifeq ($(DOCKER_BUILD), yes)
	_DOCKER_GOPATH = /go
	_DOCKER_WORKDIR = $(_DOCKER_GOPATH)/src/github.com/Mirantis/k8s-externalipcontroller/
	_DOCKER_IMAGE  = golang:1.7
	DOCKER_DEPS = apt-get update; apt-get install -y libpcap-dev;
	DOCKER_EXEC = docker run --rm -it -v "$(ROOT_DIR):$(_DOCKER_WORKDIR)" \
		-w "$(_DOCKER_WORKDIR)" $(_DOCKER_IMAGE)
else
	DOCKER_EXEC =
	DOCKER_DEPS =
endif

.PHONY: help
help:
	@echo "Usage: 'make <target>'"
	@echo ""
	@echo "Targets:"
	@echo "help                - Print this message and exit"
	@echo "get-deps            - Install project dependencies"
	@echo "containerized-build - Build ipmanager binary in container"
	@echo "build               - Build ipmanager binary"
	@echo "build-image         - Build docker image"
	@echo "test                - Run all tests"
	@echo "unit                - Run unit tests"
	@echo "integration         - Run integration tests"
	@echo "e2e                 - Run e2e tests"
	@echo "docker-publish      - Push images to Docker Hub registry"
	@echo "clean               - Delete binaries"
	@echo "clean-k8s           - Delete kubeadm-dind-cluster"
	@echo "clean-all           - Delete binaries and vendor files"

.PHONY: get-deps
get-deps: $(VENDOR_DIR)


.PHONY: build
build: $(BUILD_DIR)/ipmanager


.PHONY: containerized-build
containerized-build:
	make build DOCKER_BUILD=yes


.PHONY: build-image
build-image: $(BUILD_IMAGE_MARKER)


.PHONY: unit
unit:
	$(DOCKER_EXEC) bash -xc '$(DOCKER_DEPS) \
		go test -v ./pkg/...'


.PHONY: integration
integration: $(BUILD_DIR)/integration.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/integration.test --ginkgo.v --logtostderr --v=10


.PHONY: e2e
e2e: $(BUILD_DIR)/e2e.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/e2e.test --master=http://localhost:8080 --testlink=dind0 -ginkgo.v


.PHONY: test
test: unit integration e2e


.PHONY: docker-publish
docker-publish:
	IMAGE_REPO=$(IMAGE_REPO) IMAGE_TAG=$(IMAGE_TAG) bash ./scripts/docker_publish.sh


.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)


.PHONY: clean-k8s
clean-k8s:
	bash ./scripts/dind-cluster-$(DIND_CLUSTER_VERSION).sh clean
	rm -f ./scripts/dind-cluster-$(DIND_CLUSTER_VERSION).sh
	rm -rf $(HOME)/.kubeadm-dind-cluster
	rm -rf $(HOME)/.kube
	rm -f $(ENV_PREPARE_MARKER)


.PHONY: clean-all
clean-all: clean clean-k8s
	rm -rf $(VENDOR_DIR)
	rm -f $(BUILD_IMAGE_MARKER)
	docker rmi -f $(IMAGE_REPO):$(IMAGE_TAG)


$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)


$(VENDOR_DIR):
	$(DOCKER_EXEC) bash -xc 'go get github.com/Masterminds/glide && \
		glide install --strip-vendor; \
		chown $(shell id -u):$(shell id -g) -R vendor'


$(BUILD_DIR)/ipmanager: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) bash -xc '$(DOCKER_DEPS) \
		go build --ldflags "-extldflags \"-static\"" \
		-o $@ ./cmd/ipmanager/ ; \
		chown $(shell id -u):$(shell id -g) -R _output'


$(BUILD_DIR)/e2e.test: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) bash -xc '$(DOCKER_DEPS) \
		go test -c -o $@ ./test/e2e/'


$(BUILD_DIR)/integration.test: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) bash -xc '$(DOCKER_DEPS) \
		go test -c -o $@ ./test/integration/'


$(BUILD_IMAGE_MARKER): $(BUILD_DIR)/ipmanager
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	touch $(BUILD_IMAGE_MARKER)


$(ENV_PREPARE_MARKER): build-image
	DIND_CLUSTER_VERSION=$(DIND_CLUSTER_VERSION) bash ./scripts/kubeadm_dind_cluster.sh
	IMAGE_REPO=$(IMAGE_REPO) IMAGE_TAG=$(IMAGE_TAG) bash ./scripts/import_images.sh
	touch $(ENV_PREPARE_MARKER)
