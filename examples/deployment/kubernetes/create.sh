#!/usr/bin/env bash
#
# This script (optionally) creates and then prepares a Google Cloud project to host a
# Trillian instance using Kubernetes.

function checkEnv() {
  if [ -z ${PROJECT_ID+x} ] ||
     [ -z ${CLUSTER_NAME+x} ] ||
     [ -z ${REGION+x} ] ||
     [ -z ${NODE_LOCATIONS+x} ] ||
     [ -z ${MASTER_ZONE+x} ] ||
     [ -z ${CONFIGMAP+x} ] ||
     [ -z ${POOLSIZE+x} ] ||
     [ -z ${MACHINE_TYPE+x} ]; then
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
if ! envsubst --help > /dev/null; then
  echo "Please install the envsubt command"
  exit 1
fi

echo "Creating new Trillian deployment"
echo "  Project name: ${PROJECT_ID}"
echo "  Cluster name: ${CLUSTER_NAME}"
echo "  Region:       ${REGION}"
echo "  Node Locs:    ${NODE_LOCATIONS}"
echo "  Config:       ${CONFIGMAP}"
echo "  Pool:         ${POOLSIZE} * ${MACHINE_TYPE}"
echo

# Uncomment this to create a GCE project from scratch, or you can create it
# manually through the web UI.
# gcloud projects create ${PROJECT_ID}

# Connect to gcloud
gcloud config set project "${PROJECT_ID}"
gcloud config set compute/zone ${MASTER_ZONE}
gcloud config set container/cluster "${CLUSTER_NAME}"

# Ensure Kubernetes Engine (container) and Cloud Spanner (spanner) services are enabled
for SERVICE in container spanner; do
  gcloud services enable ${SERVICE}.googleapis.com --project=${PROJECT_ID}
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
gcloud iam service-accounts keys create /dev/stdout --iam-account="trillian@${PROJECT_ID}.iam.gserviceaccount.com" |
  kubectl create secret generic trillian-key --from-file=key.json=/dev/stdin
# Update roles
for ROLE in spanner.databaseUser logging.logWriter monitoring.metricWriter; do
  gcloud projects add-iam-policy-binding "${PROJECT_ID}" \
    --member "serviceAccount:trillian@${PROJECT_ID}.iam.gserviceaccount.com" \
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
