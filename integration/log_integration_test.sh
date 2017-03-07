#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./server/trillian_log_server/
go build ${GOFLAGS} ./server/trillian_log_signer/

TEST_TREE_ID=1123
RPC_PORT=$(pickUnusedPort)

yes | "${SCRIPTS_DIR}"/resetdb.sh
for tid in 0 $TEST_TREE_ID; do
  echo "Provisioning test log (Tree ID: $tid) in database"
  "${SCRIPTS_DIR}"/createlog.sh ${tid}
done

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} &
RPC_SERVER_PID=$!
popd > /dev/null

# Ensure we kill the RPC server once we're done.
TO_KILL+=(${RPC_SERVER_PID})
waitForServerStartup ${RPC_PORT}

echo "Starting Log signer"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_log_signer --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --sequencer_sleep_between_runs="1s" --batch_size=100 --export_metrics=false &
LOG_SIGNER_PID=$!
TO_KILL+=(${LOG_SIGNER_PID})
popd > /dev/null

# Run the test(s):
cd "${INTEGRATION_DIR}"
set +e
go test -run ".*LiveLog.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --log_rpc_server="localhost:${RPC_PORT}"
RESULT=$?
set -e

echo "Stopping Log signer (pid ${LOG_SIGNER_PID})"
killPid ${LOG_SIGNER_PID}
echo "Stopping Log RPC server (pid ${RPC_SERVER_PID})"
killPid ${RPC_SERVER_PID}
TO_KILL=()

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_server.INFO
    echo "Signer log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_signer.INFO
    exit $RESULT
fi
