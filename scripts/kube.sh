#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

CURDIR=$(dirname "${BASH_SOURCE}")
source "${CURDIR}/config.sh"

function fetch-kube {
        cd ${WORKDIRECTORY}
        git clone https://github.com/kubernetes/kubernetes.git
        cd kubernetes/
        git checkout tags/v1.4.4
        go get -u github.com/jteeuwen/go-bindata/go-bindata
        make WHAT='cmd/hyperkube'
        make WHAT='cmd/kubectl'
}

fetch-kube
