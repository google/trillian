#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

echo "Launching core Trillian map components"
map_prep_test 1
TO_KILL+=(${RPC_SERVER_PIDS[@]})

echo "Running test"
cd "${INTEGRATION_DIR}"
set +e
TRILLIAN_SQL_DRIVER=mysql go test ${GOFLAGS} \
  -timeout=${GO_TEST_TIMEOUT:-5m} \
  ./maptest  --map_rpc_server="${RPC_SERVER_1}"
RESULT=$?
set -e

map_stop_test
TO_KILL=()

if [ $RESULT != 0 ]; then
    sleep 1
    echo "Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_map_server.INFO
    exit $RESULT
fi
