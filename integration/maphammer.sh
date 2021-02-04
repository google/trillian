#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

# Default to one map
MAP_COUNT=${1:-1}

map_prep_test 1
TO_KILL+=(${RPC_SERVER_PIDS[@]})

echo "Provisioning map"
map_provision "${RPC_SERVER_1}" ${MAP_COUNT}

metrics_port=$(pick_unused_port)
echo "Running test(s) with metrics at localhost:${metrics_port}"
set +e
go run ${GOFLAGS} github.com/google/trillian/testonly/internal/hammer/maphammer \
  --map_ids=${MAP_IDS} --admin_server=${RPC_SERVER_1} --rpc_server=${RPC_SERVER_1} --metrics_endpoint="localhost:${metrics_port}" --logtostderr ${HAMMER_OPTS}
RESULT=$?
set -e

map_stop_test
TO_KILL=()

exit $RESULT
