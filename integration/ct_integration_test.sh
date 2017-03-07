#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

# Default to one RPC server and one HTTP server.
RPC_SERVER_COUNT=${1:-1}
HTTP_SERVER_COUNT=${2:-1}

. "${INTEGRATION_DIR}"/ct_prep_test.sh "${RPC_SERVER_COUNT}" "${HTTP_SERVER_COUNT}"

# Ensure everything is tidied on exit
TO_DELETE="${TO_DELETE} ${CT_CFG}"
TO_KILL+=(${HTTP_SERVER_PIDS[@]})
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${LB_SERVER_PID})
TO_KILL+=(${RPC_SERVER_PIDS[@]})

echo "Running test(s)"
set +e
go test -v -run ".*LiveCT.*" --timeout=5m ./integration --log_config "${CT_CFG}" --ct_http_servers=${CT_SERVERS} --testdata_dir=${TESTDATA}
RESULT=$?
set -e

. "${INTEGRATION_DIR}"/ct_stop_test.sh
TO_KILL=()

exit $RESULT
