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
export LOG_RPC="http://127.0.0.1:${PORT}"

echo kubectl port-forward service/trillian-log-service ${PORT}:8090
kubectl port-forward service/trillian-log-service ${PORT}:8090 &
trap "kill %1" 0 1 2 3 4 6 9 15

sleep 3

echo "Creating tree..."
TREEID=$(go run github.com/google/trillian/cmd/createtree --admin_server=${LOG_RPC})

echo "Created tree ${TREEID}:"

# stop port-forwarding
kill %1
