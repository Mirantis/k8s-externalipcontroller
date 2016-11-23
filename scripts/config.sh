#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail
set -o xtrace

NUM_NODES=${NUM_NODES:-2}
DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
WORKDIRECTORY=${WORKDIRECTORY:-"/tmp/kube"}
mkdir -p $WORKDIRECTORY
