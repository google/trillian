#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

TRILLIAN_SERVER="$1"
TEST_STARTED_TRILLIAN_SERVER=false

if [ -z "${TRILLIAN_SERVER}" ]; then
  echo "Launching core Trillian log components"
  log_prep_test 1 1

  # Cleanup for the Trillian components
  TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
  TO_KILL+=(${LOG_SIGNER_PIDS[@]})
  TO_KILL+=(${RPC_SERVER_PIDS[@]})
  TO_KILL+=(${ETCD_PID})

  TRILLIAN_SERVER="${RPC_SERVER_1}"
  TEST_STARTED_TRILLIAN_SERVER=true
fi

echo "Provision log"
TEST_TREE_ID=$(go run github.com/google/trillian/cmd/createtree \
  --admin_server="${TRILLIAN_SERVER}" \
  ${KEY_ARGS})
echo "Created tree ${TEST_TREE_ID}"

echo "Running test"
pushd "${INTEGRATION_DIR}"
set +e
go test \
  -run ".*LiveLog.*" \
  -timeout=${GO_TEST_TIMEOUT:-5m} \
  ./ \
  --log_rpc_server="${TRILLIAN_SERVER}" \
  --treeid ${TEST_TREE_ID}
RESULT=$?
set -e
popd

if ${TEST_STARTED_TRILLIAN_SERVER}; then
  log_stop_test
  TO_KILL=()

  if [ $RESULT != 0 ]; then
    sleep 1
    echo "Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_server.INFO
    echo "Signer log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_signer.INFO
  fi
fi

exit $RESULT
