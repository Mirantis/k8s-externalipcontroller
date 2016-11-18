IMAGE_REPO ?= mirantis/k8s-externalipcontroller
IMAGE_TAG ?= latest
DOCKER_BUILD ?= no

BUILD_DIR = _output
ROOT_DIR = $(abspath $(dir $(lastword $(MAKEFILE_LIST))))

ENV_PREPARE_MARKER = .env-prepare.complete
BUILD_IMAGE_MARKER = .build-image.complete

ifeq ( $(DOCKER_BUILD), yes )
	DOCKER_EXEC =
else
	_DOCKER_GOPATH = /gopath
	_DOCKER_WORKDIR = $(_DOCKER_GOPATH)/src/github.com/Mirantis/k8s-externalipcontroller/
	_DOCKER_IMAGE  = golang:1.7
	DOCKER_EXEC = docker run --rm -it -v "$(ROOT_DIR):$(_DOCKER_WORKDIR)" \
		-e "GOPATH=$(_DOCKER_GOPATH)" -w "$(_DOCKER_WORKDIR)" $(_DOCKER_IMAGE)
endif

.PHONY: help
help:
	@echo "Usage: 'make <target>'"
	@echo ""
	@echo "Targets:"
	@echo "help            - Print this message and exit"
	@echo "get-deps        - Install project dependencies"
	@echo "build           - Build ipcontroller binary"
	@echo "build-image     - Build docker image"
	@echo "test            - Run all tests"
	@echo "unit            - Run unit tests"
	@echo "integration     - Run integration tests"
	@echo "e2e             - Run e2e tests"


.PHONY: get-deps
get-deps:
	go get github.com/Masterminds/glide
	glide install --strip-vendor


.PHONY: build
build: $(BUILD_DIR)/ipcontroller


.PHONY: containerized-build
containerized-build:
	make build DOCKER_BUILD=yees


.PHONY: build-image
build-image: $(BUILD_IMAGE_MARKER)


.PHONY: unit
unit:
	$(DOCKER_EXEC) go test -v ./pkg/...


.PHONY: integration
integration: $(BUILD_DIR)/integration.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/integration.test


.PHONY: e2e
e2e: $(BUILD_DIR)/e2e.test $(ENV_PREPARE_MARKER)
	sudo $(BUILD_DIR)/e2e.test --master=http://localhost:8888 --testlink=docker0 -ginkgo.v


.PHONY: test
test: unit integration e2e


.PHONY: clean
clean:
	rm -rf $(BUILD_DIR)


$(BUILD_DIR):
	mkdir -p $(BUILD_DIR)


$(BUILD_DIR)/ipcontroller: $(BUILD_DIR)
	$(DOCKER_EXEC) go build -o $@ cmd/ipcontroller.go


$(BUILD_DIR)/e2e.test:
	$(DOCKER_EXEC) go test -c -o $@ ./test/e2e/


$(BUILD_DIR)/integration.test: $(BUILD_DIR)
	$(DOCKER_EXEC) go test -c -o $@ ./test/integration/


$(BUILD_IMAGE_MARKER): $(BUILD_DIR)/ipcontroller
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .
	echo > $(BUILD_IMAGE_MARKER)


$(ENV_PREPARE_MARKER): build-image
	./prepare.sh
	CONTAINER_ID=$$(docker create $(IMAGE_REPO):$(IMAGE_TAG) bash) && \
		docker export $$CONTAINER_ID > $(BUILD_DIR)/ipcontroller.tar
	docker cp $(BUILD_DIR)/ipcontroller.tar dind_node_1:/tmp
	docker exec -ti dind_node_1 docker import /tmp/ipcontroller.tar $(IMAGE_REPO):$(IMAGE_TAG)
	docker cp $(BUILD_DIR)/ipcontroller.tar dind_node_2:/tmp
	docker exec -ti dind_node_2 docker import /tmp/ipcontroller.tar $(IMAGE_REPO):$(IMAGE_TAG)
	echo > $(ENV_PREPARE_MARKER)
