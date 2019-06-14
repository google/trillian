DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export NAMESPACE=default
export PROJECT_ID=trillian-opensource-ci
export CLUSTER_NAME=trillian-opensource-ci
export REGION=us-central1
export MASTER_ZONE="${REGION}-a"
export NODE_LOCATIONS="${REGION}-a,${REGION}-c"
export CONFIGMAP=${DIR}/trillian-cloudspanner.yaml

export POOLSIZE=10
export MACHINE_TYPE="n1-standard-4"

export RUN_MAP=false
