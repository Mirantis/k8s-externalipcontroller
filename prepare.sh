#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

NUM_NODES=${NUM_NODES:-2}
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
WORKDIRECTORY=${WORKDIRECTORY:-}

ETCD_ENDPOINT="http://localhost:4001"

function fetch-kube {
        git clone https://github.com/kubernetes/kubernetes.git
        cd kubernetes/
        git checkout tags/v1.4.4
        go get -u github.com/jteeuwen/go-bindata/go-bindata
        make WHAT='cmd/hyperkube'
        make WHAT='cmd/kubectl'
}

function prepare-dind-cluster {
        git clone https://github.com/sttts/kubernetes-dind-cluster.git dind
        NUM_NODES="${NUM_NODES}" dind/dind-up-cluster.sh
        for i in `seq 1 "${NUM_NODES}"`;
        do
          docker exec -ti dind_node_$i ip l set docker0 promisc on
        done
}

function wait-kube {
    local attempts=5
    while [[ $attempts -gt 0 ]] && \
            curl --connection-timeout 60 --silent --head \
            "$ETCD_ENDPOINT" > /dev/null; do
        (( -- attempts ))
    done
}

if [ -z "$WORKDIRECTORY" ]; then
        WORKDIRECTORY="$(mktemp -d -t ipcontrollerXXXX)"
fi
cd ${WORKDIRECTORY}
fetch-kube
cd ${WORKDIRECTORY}/kubernetes
prepare-dind-cluster
wait-kube
cd ${DIR}
