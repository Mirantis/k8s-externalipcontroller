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
        mkdir -p _output/
        docker export "${CONTAINERID}" > _output/ipcontroller.tar
        # TODO implement it as a provider (e.g source functions)
        docker cp _output/ipcontroller.tar dind_node_1:/tmp
        # docker exec -ti dind_node_1 docker rmi -f ${IMAGE_REPO}:${IMAGE_TAG}
        docker exec -ti dind_node_1 docker import /tmp/ipcontroller.tar ${IMAGE_REPO}:${IMAGE_TAG}
        docker cp _output/ipcontroller.tar dind_node_2:/tmp
        #docker exec -ti dind_node_1 docker rmi -f ${IMAGE_REPO}:${IMAGE_TAG}
        docker exec -ti dind_node_2 docker import /tmp/ipcontroller.tar ${IMAGE_REPO}:${IMAGE_TAG}
        set +o xtrace
        echo "Finished copying docker image to dind nodes"
}

function run-tests {
        echo "Running e2e tests"
        set -o xtraceo
        go test -c -o _output/e2e.test ./test/e2e/
        sudo ./_output/e2e.test --master=http://localhost:8888 --testlink=docker0 -ginkgo.v
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
