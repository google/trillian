#!/usr/bin/env bash
#set -o pipefail
#set -o errexit
#set -o nounset
#set -o xtrace

export PROJECT_NAME_CI=trillian-opensource-ci
export CLUSTER_NAME_CI=trillian-opensource-ci
export CLOUDSDK_COMPUTE_ZONE=us-central1-a


gcloud --quiet config set project ${PROJECT_NAME_CI}
gcloud --quiet config set container/cluster ${CLUSTER_NAME_CI}
gcloud --quiet config set compute/zone ${CLOUDSDK_COMPUTE_ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME_CI}


echo "Building docker images..."
cd $GOPATH/src/github.com/google/trillian
docker build --quiet -f examples/deployment/docker/log_server/Dockerfile -t gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT} .
docker build --quiet -f examples/deployment/docker/log_signer/Dockerfile -t gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT} .

echo "Pushing docker images..."
gcloud docker -- push gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT}
gcloud docker -- push gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT}

echo "Tagging docker images..."
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME_CI}/log_server:latest
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME_CI}/log_signer:latest

echo "Updating jobs..."
kubectl delete configmap deploy-config
kubectl create -f examples/deployment/kubernetes/trillian-opensource-ci.yaml

kubectl apply -f examples/deployment/kubernetes/trillian-log-deployment.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-service.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-signer-deployment.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-signer-service.yaml
kubectl set image deployment/trillian-logserver-deployment trillian-logserver=gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT}
kubectl set image deployment/trillian-logsigner-deployment trillian-logsigner=gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT}
