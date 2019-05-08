#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

TRILLIAN_SERVER="$1"
TEST_STARTED_TRILLIAN_SERVER=false

if [ -z "${TRILLIAN_SERVER}" ]; then
  echo "Launching core Trillian map components"
  map_prep_test 1
  TO_KILL+=(${RPC_SERVER_PIDS[@]})

  TRILLIAN_SERVER="${RPC_SERVER_1}"
  TEST_STARTED_TRILLIAN_SERVER=true
fi

echo "Running test"
cd "${INTEGRATION_DIR}"
set +e
go test \
  -timeout=${GO_TEST_TIMEOUT:-5m} \
  ./maptest  --map_rpc_server="${TRILLIAN_SERVER}"
RESULT=$?
set -e

if $TEST_STARTED_TRILLIAN_SERVER; then
  map_stop_test
  TO_KILL=()

  if [ $RESULT != 0 ]; then
      sleep 1
      echo "Server log:"
      echo "--------------------"
      cat "${TMPDIR}"/trillian_map_server.INFO
  fi
fi

exit $RESULT
