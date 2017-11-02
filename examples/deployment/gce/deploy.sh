#!/bin/sh
# Script assumptions:
# - Go 1.9 is installed
export PS1=$
export PROJECT_NAME=kt-hackathon
export TAG=gbelvin
export LOG_URL=http://104.198.78.25:8094
export MAP_URL=http://35.193.30.82:8094

# Get Trillian
go get github.com/google/trillian/...
go get github.com/google/certificate-transparency-go/...

cd $GOPATH/src/github.com/google/trillian

# Connect to gcloud
gcloud config set project $PROJECT_NAME
gcloud config set compute/zone us-central1-b
gcloud beta container clusters create "cluster-1" --machine-type "n1-standard-1" --image-type "COS" --num-nodes "5" 
gcloud container clusters get-credentials cluster-1

# Build docker images
docker build -f examples/deployment/docker/db_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/db:$TAG .
docker build -f examples/deployment/docker/log_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_server:$TAG .
docker build -f examples/deployment/docker/log_signer/Dockerfile -t us.gcr.io/$PROJECT_NAME/log_signer:$TAG .
docker build -f examples/deployment/docker/map_server/Dockerfile -t us.gcr.io/$PROJECT_NAME/map_server:$TAG .
docker build -f trillian/examples/deployment/docker/ctfe/Dockerfile -t us.gcr.io/$PROJECT_NAME/ctfe:$TAG .

# Push docker images?
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/db:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/log_server:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/log_signer:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/map_server:$TAG
gcloud docker -- push us.gcr.io/${PROJECT_NAME}/ctfe:$TAG

# Prepare secrets

# Launch with kubernetes
#~/bin/templater.sh examples/deployment/kubernetes/trillian-deployment.yml > examples/deployment/kubernetes/trillian-demo.yml
kubectl replace -f examples/deployment/kubernetes/.
kubectl get all
kubectl get services

# Create tree
curl -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }'

# Add something to the tree

