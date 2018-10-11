#!/usr/bin/env bash
#
# This script (optionally) creates and then prepares a Google Cloud project to host a
# Trillian instance using Kubernetes.

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
source ${DIR}/config.sh

# Check required binaries are installed
if ! gcloud --help > /dev/null; then
  echo "Need gcloud installed."
  exit 1
fi
if ! kubectl --help > /dev/null; then
  echo "Need kubectl installed."
  exit 1
fi
if ! jq --help > /dev/null; then
  echo "Please install the jq command"
  exit 1
fi

echo "Creating new Trillian deployment"
echo "  Project name: ${PROJECT_NAME}"
echo "  Cluster name: ${CLUSTER_NAME}"
echo "  Region:       ${REGION}"
echo "  Node Locs:    ${NODE_LOCATIONS}"
echo "  Config:       ${CONFIGMAP}"
echo "  Pool:         ${POOLSIZE} * ${MACHINE_TYPE}"
echo

# Uncomment this to create a GCE project from scratch, or you can create it
# manually through the web UI.
# gcloud projects create ${PROJECT_NAME}

# Connect to gcloud
gcloud config set project "${PROJECT_NAME}"
gcloud config set compute/zone ${MASTER_ZONE}
gcloud config set container/cluster "${CLUSTER_NAME}"

# Ensure Kubernetes Engine (container) and Cloud Spanner (spanner) services are enabled
for SERVICE in container spanner; do
  gcloud services enable ${SERVICE}.googleapis.com --project=${PROJECT_NAME}
done

# Create cluster
# TODO(https://github.com/google/trillian/issues/1183): Add support for priorities and preemption when Kubernetes 1.11 is GA.
gcloud container clusters create "${CLUSTER_NAME}" --machine-type "${MACHINE_TYPE}" --image-type "COS" --num-nodes "${POOLSIZE}" --enable-autorepair --enable-autoupgrade --node-locations="${NODE_LOCATIONS}"
gcloud container clusters get-credentials "${CLUSTER_NAME}"

# Create spanner instance & DB
gcloud spanner instances create trillian-spanner --description "Trillian Spanner instance" --nodes=1 --config="regional-${REGION}"
gcloud spanner databases create trillian-db --instance trillian-spanner --ddl="$(cat ${DIR}/../../../storage/cloudspanner/spanner.sdl | grep -v '^--.*$')"

# Create service account
gcloud iam service-accounts create trillian --display-name "Trillian service account"
# Get the service account key and push it into a Kubernetes secret:
gcloud iam service-accounts keys create /dev/stdout --iam-account="trillian@${PROJECT_NAME}.iam.gserviceaccount.com" |
  kubectl create secret generic trillian-key --from-file=key.json=/dev/stdin
# Update roles
for ROLE in spanner.databaseUser logging.logWriter monitoring.metricWriter; do
  gcloud projects add-iam-policy-binding "${PROJECT_NAME}" \
    --member "serviceAccount:trillian@${PROJECT_NAME}.iam.gserviceaccount.com" \
    --role "roles/${ROLE}"
done

# Bring up etcd cluster
# Work-around for etcd-operator role on GKE.
COREACCOUNT=$(gcloud config config-helper --format=json | jq -r '.configuration.properties.core.account')
kubectl create clusterrolebinding etcd-cluster-admin-binding --clusterrole=cluster-admin --user="${COREACCOUNT}"

kubectl apply -f ${DIR}/etcd-role-binding.yaml
kubectl apply -f ${DIR}/etcd-role.yaml
kubectl apply -f ${DIR}/etcd-deployment.yaml
kubectl apply -f ${DIR}/etcd-service.yaml

# TODO(al): wait for this properly somehow
sleep 30

# TODO(al): have to wait before doing this?
kubectl apply -f ${DIR}/etcd-cluster.yaml
