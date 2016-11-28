#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

function push-to-docker {
    if [ "$TRAVIS_PULL_REQUEST_BRANCH" != "" ]; then
         echo "Processing PR $TRAVIS_PULL_REQUEST_BRANCH"
         exit 0
    fi

    docker login -u="$DOCKER_USERNAME" -p="$DOCKER_PASSWORD"

    set -o xtrace
    local branch=$TRAVIS_BRANCH
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
