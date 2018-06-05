#!/usr/bin/env bash
# Script assumptions:
# - Cluster has already been created & configured using the create.sh script
# - Go 1.10 is installed

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${DIR}/config.sh

export TAG=$(git rev-parse HEAD)

cd $GOPATH/src/github.com/google/trillian

envsubst < examples/deployment/kubernetes/trillian-cloudspanner.yaml | kubectl delete -f -

# Delete log-[service|signer]
envsubst < examples/deployment/kubernetes/trillian-log-deployment.yaml | kubectl delete -f -
envsubst < examples/deployment/kubernetes/trillian-log-service.yaml | kubectl delete -f -
envsubst < examples/deployment/kubernetes/trillian-log-signer-deployment.yaml | kubectl delete -f -
envsubst < examples/deployment/kubernetes/trillian-log-signer-service.yaml | kubectl delete -f -
