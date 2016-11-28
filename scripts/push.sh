#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

function push-to-docker {
    docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"

    set -o xtrace
    local branch=`git rev-parse --abbrev-ref HEAD`
    echo "Using git branch $branch"

    if [ $branch == "master" ]; then
        echo "Pushing with tag - latest"
        docker push mirantis/k8s-externalipcontroller:latest;
    fi

    if [ "${branch:0:8}" == "release-" ]; then
        echo "Pushing from release branch with tag - $branch"
        docker tag mirantis/k8s-externalipcontroller mirantis/k8s-externalipcontroller:$branch;
        docker push mirantis/k8s-externalipcontroller:$branch;
    fi
}

push-to-docker
