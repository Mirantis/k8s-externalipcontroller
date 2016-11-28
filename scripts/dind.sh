#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

CURDIR=$(dirname "${BASH_SOURCE}")
source "${CURDIR}/config.sh"

DIND_IMAGE="k8s.io/kubernetes-dind"
DIND_TAR="$WORKDIRECTORY/dind.tar"

function prepare-dind-cluster {
        cd ${WORKDIRECTORY}/kubernetes
        if [ ! -d "dind" ]; then
             git clone https://github.com/sttts/kubernetes-dind-cluster.git dind
        fi
        if [ -f $DIND_TAR ]; then
           docker import $DIND_TAR $DIND_IMAGE
        fi
        NUM_NODES="${NUM_NODES}" dind/dind-down-cluster.sh
        NUM_NODES="${NUM_NODES}" dind/dind-up-cluster.sh
        for i in `seq 1 "${NUM_NODES}"`;
        do
          docker exec -ti dind_node_$i ip l set docker0 promisc on
        done
        echo "Saving $DIND_IMAGE into tar $DIND_TAR"
        docker save $DIND_IMAGE > $DIND_TAR
}

prepare-dind-cluster
