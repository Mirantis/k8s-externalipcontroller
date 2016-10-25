#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
WORKDIRECTORY=${WORKDIRECTORY:-}

function fetch-kube {
	git clone https://github.com/kubernetes/kubernetes.git
	cd kubernetes/
	go get -u github.com/jteeuwen/go-bindata/go-bindata
	make WHAT='cmd/hyperkube'
	make WHAT='cmd/kubectl'
}

function prepare-dind-cluster {
	git clone https://github.com/sttts/kubernetes-dind-cluster.git dind
	NUM_NODES=1 dind/dind-up-cluster.sh
}

if [ -z "$WORKDIRECTORY" ]; then
	WORKDIRECTORY="$(mktemp -d -t ipcontrollerXXXX)"
fi
cd ${WORKDIRECTORY}
fetch-kube
prepare-dind-cluster
cd ${DIR}