#!/usr/bin/env bash
# Script assumptions:
# - Cluster has already been created & configured using the create.sh script
# - Go 1.9 is installed

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${DIR}/config.sh

export LOG_URL=TODO
export MAP_URL=TODO

# Get Trillian
go get github.com/google/trillian/...
cd $GOPATH/src/github.com/google/trillian

# Build docker images
docker build -f examples/deployment/docker/db_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/db:$TAG .
docker build -f examples/deployment/docker/log_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_server:$TAG .
docker build -f examples/deployment/docker/log_signer/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_signer:$TAG .
docker build -f examples/deployment/docker/map_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/map_server:$TAG .

# Connect to gcloud
gcloud config set project "${PROJECT_NAME}"
gcloud config set compute/zone "${ZONE}"

# Push docker images
gcloud docker -- push "us.gcr.io/${PROJECT_NAME}/db:${TAG}"
gcloud docker -- push "us.gcr.io/${PROJECT_NAME}/log_server:${TAG}"
gcloud docker -- push "us.gcr.io/${PROJECT_NAME}/log_signer:${TAG}"
gcloud docker -- push "us.gcr.io/${PROJECT_NAME}/map_server:${TAG}"

# Prepare configmap:
kubectl delete configmap deploy-config
envsubst < examples/deployment/kubernetes/trillian-mysql.yaml | kubectl create -f -

# Launch with kubernetes
envsubst < examples/deployment/kubernetes/trillian-log-deployment.yaml | kubectl apply -f -
envsubst < examples/deployment/kubernetes/trillian-log-service.yaml | kubectl apply -f -
envsubst < examples/deployment/kubernetes/trillian-log-signer-deployment.yaml | kubectl apply -f -
envsubst < examples/deployment/kubernetes/trillian-log-signer-service.yaml | kubectl apply -f -
kubectl get all
kubectl get services

# Create trees
curl -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
curl -X POST ${MAP_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"MAP", "hash_strategy":"CONIKS_SHA512_256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'
