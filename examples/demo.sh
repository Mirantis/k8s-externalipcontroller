#!/bin/bash

set -x

DIR=$(dirname "${BASH_SOURCE}")

echo "fresh dind cluster"
bash $DIR/../scripts/dind.sh 2> /dev/null

echo "claims controller and scheduler"
kubectl apply -f $DIR/claims/

echo "creating nginx deployment with 2 replicas + nginx service with 2 external ips"
kubectl apply -f $DIR/nginx.yaml

echo "see those externalips"
kubectl get svc nginxsvc
