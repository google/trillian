#!/usr/bin/env bash
#set -o pipefail
#set -o errexit
#set -o nounset
#set -o xtrace

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export PROJECT_ID=trillian-opensource-ci
export CLUSTER_NAME=trillian-opensource-ci
export REGION=us-central1
export MASTER_ZONE=us-central1-a
export CONFIGMAP=${DIR}/../examples/deployment/kubernetes/trillian-opensource-ci.yaml
export IMAGE_TAG=${TRAVIS_COMMIT}

${DIR}/../examples/deployment/kubernetes/deploy.sh
