#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

CURDIR=$(dirname "${BASH_SOURCE[0]}")
source "${CURDIR}/config.sh"
DESIRED_TAG="v1.4.4"

function fetch-kube {
  cd "${WORKDIRECTORY}"
  if [ ! -d "kubernetes" ]; then
    git clone https://github.com/kubernetes/kubernetes.git
  fi
  cd kubernetes/
  local current_tag=$(git describe --tags)
  if [ "$current_tag" == "$DESIRED_TAG" ]; then
    echo "Assuming that sources are already built for tag $DESIRED_TAG. Exiting..."
    exit 0
  fi
  git checkout tags/v1.4.4
  go get -u github.com/jteeuwen/go-bindata/go-bindata
  make WHAT='cmd/hyperkube'
  make WHAT='cmd/kubectl'
}

fetch-kube
