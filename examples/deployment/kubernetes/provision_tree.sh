#!/usr/bin/env bash
# Script assumptions:
# - Cluster has already been created, configured, and deployed using the
#   create.sh and deploy.sh scripts.

set -e

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

if [ $# -ne 1 ]; then
  echo "Usage: $0 <config file>"
  exit 1
fi

# Set up tunnel:
PORT=35791
export LOG_URL="http://127.0.0.1:${PORT}"

echo kubectl port-forward service/trillian-log-service ${PORT}:8091
kubectl port-forward service/trillian-log-service ${PORT}:8091 &
trap "kill %1" 0 1 2 3 4 6 9 15

sleep 3

echo "Creating tree..."
# TODO(al): use cmd/createtree instead.
TREE=$(curl -sb -X POST ${LOG_URL}/v1beta1/trees -d '{ "tree":{ "tree_state":"ACTIVE", "tree_type":"LOG", "hash_strategy":"RFC6962_SHA256", "signature_algorithm":"ECDSA", "max_root_duration":"0", "hash_algorithm":"SHA256" }, "key_spec":{ "ecdsa_params":{ "curve":"P256" } } }')

echo $TREE

TREEID=$(echo ${TREE} | jq -r .tree_id)

echo "Created tree ${TREEID}:"
echo ${TREE} | jq

echo "Initialising tree ${TREEID}:"
STH=$(curl -s -X POST ${LOG_URL}/v1beta1/logs/${TREEID}:init)
echo "Created STH:"
echo ${STH} | jq

# stop port-forwarding
kill %1
