#!/usr/bin/env bash
# Script assumptions:
# - Cluster has already been created & configured using the create.sh script
# - Go 1.9 is installed

function checkEnv() {
  if [ -z ${PROJECT_NAME+x} ] ||
     [ -z ${CLUSTER_NAME+x} ] ||
     [ -z ${MASTER_ZONE+x} ] ||
     [ -z ${CONFIGMAP+x} ]; then
    echo "You must either pass an argument which is a config file, or set all the required environment variables" >&2
    exit 1
  fi
}

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
if [ $# -eq 1 ]; then
  source $1
else
  checkEnv
fi


export LOG_URL=TODO
export MAP_URL=TODO

# if IMAGE_TAG is unset, we'll create one using the git HEAD hash, with an
# optional "-dirty" suffix if any objects have been modified.
GIT_HASH=$(git rev-parse HEAD)
GIT_DIRTY=$(git diff --quiet || echo '-dirty')
export IMAGE_TAG=${IMAGE_TAG:-${GIT_HASH}${GIT_DIRTY}}

# Connect to gcloud
gcloud --quiet config set project ${PROJECT_NAME}
gcloud --quiet config set container/cluster ${CLUSTER_NAME}
gcloud --quiet config set compute/zone ${MASTER_ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME}

# Configure Docker to use gcloud credentials with Google Container Registry
gcloud auth configure-docker

# Get Trillian
go get github.com/google/trillian/...
cd $GOPATH/src/github.com/google/trillian

echo "Building docker images..."
docker build --quiet -f examples/deployment/docker/log_server/Dockerfile -t gcr.io/$PROJECT_NAME/log_server:$IMAGE_TAG .
docker build --quiet -f examples/deployment/docker/log_signer/Dockerfile -t gcr.io/$PROJECT_NAME/log_signer:$IMAGE_TAG .
# TODO(al): when cloudspanner supports maps:
# docker build -f examples/deployment/docker/map_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/map_server:$TAG .

echo "Pushing docker images..."
gcloud docker -- push gcr.io/${PROJECT_NAME}/log_server:${IMAGE_TAG}
gcloud docker -- push gcr.io/${PROJECT_NAME}/log_signer:${IMAGE_TAG}

echo "Tagging docker images..."
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME}/log_server:${IMAGE_TAG} gcr.io/${PROJECT_NAME}/log_server:latest
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME}/log_signer:${IMAGE_TAG} gcr.io/${PROJECT_NAME}/log_signer:latest

# TODO(al): when cloudspanner supports maps:
# gcloud docker -- push "us.gcr.io/${PROJECT_NAME}/map_server:${TAG}"

echo "Updating jobs..."
# Prepare configmap:
kubectl delete configmap deploy-config || true
envsubst < ${CONFIGMAP} | kubectl create -f -

# Launch with kubernetes
envsubst < ${DIR}/trillian-log-deployment.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-service.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-signer-deployment.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-signer-service.yaml | kubectl apply -f -
kubectl set image deployment/trillian-logserver-deployment trillian-logserver=gcr.io/${PROJECT_NAME}/log_server:${IMAGE_TAG}
kubectl set image deployment/trillian-logsigner-deployment trillian-log-signer=gcr.io/${PROJECT_NAME}/log_signer:${IMAGE_TAG}
