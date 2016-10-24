#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

IMAGE_REPO=${IMAGE_REPO:-mirantis/k8s-externalipcontroller}
IMAGE_TAG=${IMAGE_TAG:-latest}

function build-image {
	# build docker image
	echo "Building docker image ${IMAGE_REPO}:${IMAGE_TAG}"
	set -o xtrace
	docker build -t ${IMAGE_REPO}:${IMAGE_TAG} "$@" .
	set +o xtrace
	echo "Built docker image ${IMAGE_REPO}:${IMAGE_TAG}"
}

function import-image {
	echo "Export docker image and import it on a dind node dind_node_1"
	CONTAINERID="$(docker create ${IMAGE_REPO}:${IMAGE_TAG} bash)"
	set -o xtrace
	docker export "${CONTAINERID}" > ipcontroller.tar
	docker cp ipcontroller.tar dind_node_1:/tmp
	docker exec -ti dind_node_1 docker import /tmp/ipcontroller.tar ${IMAGE_REPO}:${IMAGE_TAG}
	set +o xtrace
	echo "Finished copying docker image to dind_node_1"
}

function run-tests {
	echo "Running e2e tests"
	set -o xtrace
	docker run -v="/tmp/kubeconfig/kubeconfig.yaml:/etc/kubeconfig" --privileged=true \
	-e "GOPATH=/go" -w="/go/src/github.com/Mirantis/k8s-externalipcontroller" -ti ${IMAGE_REPO}:${IMAGE_TAG} go test ./test/ --kubeconfig=/etc/kubeconfig
	set +o xtrace
}

while getopts ":bir" opt; do
  case $opt in
    b)
      build-image
      ;;
    i)
      import-image
      ;;
    r)
      run-tests
      ;;
    \?)
      echo "Invalid option: -$OPTARG" >&2
      ;;
  esac
done