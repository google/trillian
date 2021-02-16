DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Set the following variables to the correct values for your Google Cloud
# project and Kubernetes cluster.
export PROJECT_ID="GOOGLE CLOUD PROJECT ID HERE (https://cloud.google.com/resource-manager/docs/creating-managing-projects#identifying_projects)"
export CLUSTER_NAME=trillian
export REGION=us-east4
export MASTER_ZONE="${REGION}-a"
export NODE_LOCATIONS="${REGION}-a,${REGION}-b,${REGION}-c"
export CONFIGMAP=${DIR}/trillian-cloudspanner.yaml
export NAMESPACE=default

export POOLSIZE=2
export MACHINE_TYPE="n1-standard-2"
