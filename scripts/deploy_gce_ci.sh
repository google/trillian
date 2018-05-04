#!/usr/bin/env bash
#set -o pipefail
#set -o errexit
#set -o nounset
#set -o xtrace

export PROJECT_NAME=trillian-opensource-lowlatency
export CLUSTER_NAME=trillian-opensource-lowlatency
export REGION=us-central1
export ZONE=us-central1-b
export CONFIGMAP=trillian-opensource-ci.yaml


gcloud --quiet config set project ${PROJECT_NAME}
gcloud --quiet config set container/cluster ${CLUSTER_NAME}
gcloud --quiet config set compute/zone ${ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME}


echo "Building docker images..."
cd $GOPATH/src/github.com/google/trillian
docker build --quiet -f examples/deployment/docker/log_server/Dockerfile -t gcr.io/${PROJECT_NAME}/log_server:${TRAVIS_COMMIT} .
docker build --quiet -f examples/deployment/docker/log_signer/Dockerfile -t gcr.io/${PROJECT_NAME}/log_signer:${TRAVIS_COMMIT} .

echo "Pushing docker images..."
gcloud docker -- push gcr.io/${PROJECT_NAME}/log_server:${TRAVIS_COMMIT}
gcloud docker -- push gcr.io/${PROJECT_NAME}/log_signer:${TRAVIS_COMMIT}

echo "Tagging docker images..."
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME}/log_server:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME}/log_server:latest
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME}/log_signer:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME}/log_signer:latest

echo "Updating jobs..."
kubectl delete configmap deploy-config
envsubst < ${DIR}/${CONFIGMAP} | kubectl create -f -

envsubst < ${DIR}/trillian-log-deployment.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-service.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-signer-deployment.yaml | kubectl apply -f -
envsubst < ${DIR}/trillian-log-signer-service.yaml | kubectl apply -f -
kubectl set image deployment/trillian-logserver-deployment trillian-logserver=gcr.io/${PROJECT_NAME}/log_server:${TRAVIS_COMMIT}
kubectl set image deployment/trillian-logsigner-deployment trillian-log-signer=gcr.io/${PROJECT_NAME}/log_signer:${TRAVIS_COMMIT}
