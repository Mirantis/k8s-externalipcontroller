IMAGE_REPO ?= mirantis/k8s-externalipcontroller
IMAGE_TAG ?= latest
DOCKER_BUILD ?= no

BUILD_DIR = _output
VENDOR_DIR = vendor
ROOT_DIR = $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

ENV_PREPARE_MARKER = .env-prepare.complete
BUILD_IMAGE_MARKER = .build-image.complete

ifeq ($(DOCKER_BUILD), yes)
	_DOCKER_GOPATH = /go
	_DOCKER_WORKDIR = $(_DOCKER_GOPATH)/src/github.com/Mirantis/k8s-externalipcontroller/
	_DOCKER_IMAGE  = golang:1.7
	DOCKER_EXEC = docker run --rm -it -v "$(ROOT_DIR):$(_DOCKER_WORKDIR)" \
		-w "$(_DOCKER_WORKDIR)" $(_DOCKER_IMAGE)
else
	DOCKER_EXEC =
endif

.PHONY: help
help:
	@echo "Usage: 'make <target>'"
	@echo ""
	@echo "Targets:"
	@echo "help            - Print this message and exit"
	@echo "get-deps        - Install project dependencies"
	@echo "containerized-build - Build ipmanager binary in container"
	@echo "build           - Build ipmanager binary"
	@echo "build-image     - Build docker image"
	@echo "test            - Run all tests"
	@echo "unit            - Run unit tests"
	@echo "integration     - Run integration tests"
	@echo "e2e             - Run e2e tests"
	@echo "clean           - Delete binaries"
	@echo "clean-all       - Delete binaries and vendor files"

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
	$(DOCKER_EXEC) go test -v ./pkg/...


.PHONY: integration
integration: $(BUILD_DIR)/integration.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/integration.test --ginkgo.v --logtostderr --v=10


.PHONY: e2e
e2e: $(BUILD_DIR)/e2e.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/e2e.test --master=http://localhost:8888 --testlink=docker0 -ginkgo.v


.PHONY: test
test: unit integration e2e


.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)


.PHONY: clean-all
clean-all: clean
	rm -rf $(VENDOR_DIR)
	rm -f $(BUILD_IMAGE_MARKER)
	docker rmi -f $(IMAGE_REPO):$(IMAGE_TAG)


$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)


$(BUILD_DIR)/ipmanager: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) go build -o $@ ./cmd/ipmanager/


$(BUILD_DIR)/e2e.test: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) go test -c -o $@ ./test/e2e/


$(BUILD_DIR)/integration.test: $(BUILD_DIR) $(VENDOR_DIR)
	$(DOCKER_EXEC) go test -c -o $@ ./test/integration/


$(BUILD_IMAGE_MARKER): $(BUILD_DIR)/ipmanager
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	echo > $(BUILD_IMAGE_MARKER)


$(VENDOR_DIR):
	$(DOCKER_EXEC) bash -xc 'go get github.com/Masterminds/glide && \
		glide install --strip-vendor; \
		chown $(shell id -u):$(shell id -g) -R vendor'


$(ENV_PREPARE_MARKER): build-image
	./prepare.sh
	CONTAINER_ID=$$(docker create $(IMAGE_REPO):$(IMAGE_TAG) bash) && \
		docker export $$CONTAINER_ID > $(BUILD_DIR)/ipcontroller.tar
	docker cp $(BUILD_DIR)/ipcontroller.tar dind_node_1:/tmp
	docker exec -ti dind_node_1 docker import /tmp/ipcontroller.tar $(IMAGE_REPO):$(IMAGE_TAG)
	docker cp $(BUILD_DIR)/ipcontroller.tar dind_node_2:/tmp
	docker exec -ti dind_node_2 docker import /tmp/ipcontroller.tar $(IMAGE_REPO):$(IMAGE_TAG)
	echo > $(ENV_PREPARE_MARKER)
