#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

# Default to one of everything.
RPC_SERVER_COUNT=${1:-1}
LOG_SIGNER_COUNT=${2:-1}
HTTP_SERVER_COUNT=${3:-1}

. "${INTEGRATION_DIR}"/ct_prep_test.sh "${RPC_SERVER_COUNT}" "${LOG_SIGNER_COUNT}" "${HTTP_SERVER_COUNT}"

# Cleanup for the Trillian components
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${LB_SERVER_PID})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})

# Cleanup for the personality
TO_DELETE="${TO_DELETE} ${CT_CFG}"
TO_KILL+=(${CT_SERVER_PIDS[@]})

echo "Running test(s)"
set +e
go test -v -run ".*LiveCT.*" --timeout=5m ./integration --log_config "${CT_CFG}" --ct_http_servers=${CT_SERVERS} --testdata_dir=${TESTDATA}
RESULT=$?
set -e

. "${INTEGRATION_DIR}"/ct_stop_test.sh
TO_KILL=()

exit $RESULT
