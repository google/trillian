#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

echo "Launching core Trillian log components"
log_prep_test 1 1
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})

TRILLIAN_SERVER="${RPC_SERVER_1}"

metrics_port=$(pick_unused_port ${port})
echo "Running test against ephemeral tree, metrics on http://localhost:${metrics_port}/metrics"
go run ${GOFLAGS} github.com/google/trillian/testonly/mdm/mdmtest \
  --rpc_server="${TRILLIAN_SERVER}" \
  --metrics_endpoint="localhost:${metrics_port}" \
  --checkers=10 \
  --new_leaf_chance=90 \
  --logtostderr
RESULT=$?
set -e

log_stop_test
TO_KILL=()
exit $RESULT
