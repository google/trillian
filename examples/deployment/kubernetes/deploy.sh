#!/usr/bin/env bash
# Script assumptions:
# - Cluster has already been created & configured using the create.sh script
# - Go 1.9 is installed

function checkEnv() {
  if [ -z ${PROJECT_ID+x} ] ||
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


# if IMAGE_TAG is unset, we'll create one using the git HEAD hash, with an
# optional "-dirty" suffix if any objects have been modified.
GIT_HASH=$(git rev-parse HEAD)
GIT_DIRTY=$(git diff --quiet || echo '-dirty')
export IMAGE_TAG=${IMAGE_TAG:-${GIT_HASH}${GIT_DIRTY}}

# Connect to gcloud
gcloud --quiet config set project ${PROJECT_ID}
gcloud --quiet config set container/cluster ${CLUSTER_NAME}
gcloud --quiet config set compute/zone ${MASTER_ZONE}
gcloud --quiet container clusters get-credentials ${CLUSTER_NAME}

# Configure Docker to use gcloud credentials with Google Container Registry
gcloud auth configure-docker

# Get Trillian
go get github.com/google/trillian/...
cd $GOPATH/src/github.com/google/trillian

images="log_server log_signer"
if ${RUN_MAP}; then
  images+=" map_server"
fi
echo "Building and pushing docker images: ${images}"
for thing in ${images}; do
  echo "  - ${thing}"
  docker build --quiet -f examples/deployment/docker/${thing}/Dockerfile -t gcr.io/$PROJECT_ID/${thing}:$IMAGE_TAG .
  docker push gcr.io/${PROJECT_ID}/${thing}:${IMAGE_TAG}
  gcloud --quiet container images add-tag gcr.io/${PROJECT_ID}/${thing}:${IMAGE_TAG} gcr.io/${PROJECT_ID}/${thing}:latest
done

echo "Updating jobs..."
# Prepare configmap:
kubectl delete --namespace="${NAMESPACE}" configmap deploy-config || true
envsubst < ${CONFIGMAP} | kubectl create --namespace="${NAMESPACE}" -f -

# Launch with kubernetes
kubeconfigs="trillian-log-deployment.yaml trillian-log-service.yaml trillian-log-signer-deployment.yaml trillian-log-signer-service.yaml"
if ${RUN_MAP}; then
  kubeconfigs+=" trillian-map-deployment.yaml trillian-map-service.yaml"
fi
for thing in ${kubeconfigs}; do
  echo ${thing}
  envsubst < ${DIR}/${thing} | kubectl apply --namespace="${NAMESPACE}" -f -
done

kubectl get all --namespace="${NAMESPACE}"
kubectl get services --namespace="${NAMESPACE}"
