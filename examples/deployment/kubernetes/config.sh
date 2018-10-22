DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

export PROJECT_ID=trillian-ct-log
export CLUSTER_NAME=trillian
export REGION=us-east4
export MASTER_ZONE="${REGION}-a"
export NODE_LOCATIONS="${REGION}-a,${REGION}-b,${REGION}-c"
export CONFIGMAP=${DIR}/trillian-cloudspanner.yaml

export POOLSIZE=2
export MACHINE_TYPE="n1-standard-2"

export RUN_MAP=false
