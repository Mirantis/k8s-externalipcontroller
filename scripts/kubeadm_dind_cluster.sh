#!/bin/bash
# Copyright 2017 Mirantis
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -o xtrace
set -o pipefail
set -o errexit
set -o nounset


NUM_NODES=${NUM_NODES:-2}
KUBEADM_SCRIPT_URL=${KUBEADM_SCRIPT_URL:-https://cdn.rawgit.com/Mirantis/kubeadm-dind-cluster/master/fixed/dind-cluster}
# kubeadm-dind-cluster supports k8s versions:
# "v1.4", "v1.5" and "v1.6".
DIND_CLUSTER_VERSION=${DIND_CLUSTER_VERSION:-v1.4}
SLAVE_NAME=${SLAVE_NAME:-"kube-node-"}


function kubeadm-dind-cluster {
  pushd "./scripts" &> /dev/null
  wget "${KUBEADM_SCRIPT_URL}-${DIND_CLUSTER_VERSION}.sh"
  chmod +x ./dind-cluster-"${DIND_CLUSTER_VERSION}".sh
  NUM_NODES="${NUM_NODES}" bash ./dind-cluster-"${DIND_CLUSTER_VERSION}".sh down
  NUM_NODES="${NUM_NODES}" bash ./dind-cluster-"${DIND_CLUSTER_VERSION}".sh up
  for node in $(seq 1 "${NUM_NODES}"); do
    docker exec -ti "${SLAVE_NAME}""${node}" ip l set dind0 promisc on
  done
  popd &> /dev/null
}

kubeadm-dind-cluster