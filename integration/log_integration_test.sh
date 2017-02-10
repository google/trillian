#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./server/trillian_log_server/

TEST_TREE_ID=1123
RPC_PORT=34557

yes | "${SCRIPTS_DIR}"/resetdb.sh
for tid in 0 $TEST_TREE_ID; do
  echo "Provisioning test log (Tree ID: $tid) in database"
  "${SCRIPTS_DIR}"/createlog.sh ${tid}
done

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Ensure we kill the RPC server once we're done.
TO_KILL+=(${RPC_SERVER_PID})
waitForServerStartup ${RPC_PORT}

# Run the test(s):
cd "${INTEGRATION_DIR}"
set +e
go test -run ".*Log.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --log_rpc_server="localhost:${RPC_PORT}"
RESULT=$?
set -e

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
    exit $RESULT
fi
