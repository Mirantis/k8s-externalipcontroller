#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

CURDIR=$(dirname "${BASH_SOURCE[0]}")
source "${CURDIR}/config.sh"

IMAGE_REPO=${IMAGE_REPO:-mirantis/k8s-externalipcontroller}
IMAGE_TAG=${IMAGE_TAG:-latest}

function import-image {
  echo "Export docker image and import it on a dind node dind_node_1"
  CONTAINERID="$(docker create "${IMAGE_REPO}":"${IMAGE_TAG}" bash)"
  mkdir -p _output/
  docker export "${CONTAINERID}" > _output/ipcontroller.tar
  # TODO implement it as a provider (e.g source functions)

  for i in $(seq 1 "${NUM_NODES}"); do
    docker cp _output/ipcontroller.tar dind_node_"$i":/tmp
    docker exec -ti dind_node_"$i" docker import /tmp/ipcontroller.tar "${IMAGE_REPO}":"${IMAGE_TAG}"
  done
  echo "Finished copying docker image to dind nodes"
}

import-image
