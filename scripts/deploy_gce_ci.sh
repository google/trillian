#!/usr/bin/env bash
set -o pipefail
set -o errexit
set -o nounset
set -o xtrace

if `false`; then
echo $GCLOUD_SERVICE_KEY_CI | base64 --decode -i > ${HOME}/gcloud-service-key.json
gcloud auth activate-service-account --key-file ${HOME}/gcloud-service-key.json

gcloud --quiet config set project ${PROJECT_NAME_CI}
gcloud --quiet config set container/cluster ${CLUSTER_NAME_CI}
gcloud --quiet config set compute/zone ${CLOUDSDK_COMPUTE_ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME_CI}
kubectl config view
kubectl config current-context


cd $GOPATH/src/github.com/google/trillian
docker build -f examples/deployment/docker/log_server/Dockerfile -t gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT} .
docker build -f examples/deployment/docker/log_signer/Dockerfile -t gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT} .

gcloud docker -- push gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT}
gcloud docker -- push gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT}
fi

gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME_CI}/log_server:latest
gcloud --quiet container images add-tag gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT} gcr.io/${PROJECT_NAME_CI}/log_signer:latest


kubectl apply -f examples/deployment/kubernetes/trillian-log-deployment.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-service.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-signer-deployment.yaml
kubectl apply -f examples/deployment/kubernetes/trillian-log-signer-service.yaml
kubectl set image deployment/trillian-logserver-deployment trillian-logserver=gcr.io/${PROJECT_NAME_CI}/log_server:${TRAVIS_COMMIT}
kubectl set image deployment/trillian-logsigner-deployment trillian-logsigner=gcr.io/${PROJECT_NAME_CI}/log_signer:${TRAVIS_COMMIT}

kubectl get all
kubectl get services


