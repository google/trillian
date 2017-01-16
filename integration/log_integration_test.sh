#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

TEST_TREE_ID=1123
RPC_PORT=34557

echo "Provisioning test log (Tree ID: $TEST_TREE_ID) in database"
${SCRIPTS_DIR}/wipelog.sh ${TEST_TREE_ID}
${SCRIPTS_DIR}/createlog.sh ${TEST_TREE_ID}

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the RPC server once we're done.
trap "kill -INT ${RPC_SERVER_PID}" EXIT
waitForServerStartup ${RPC_PORT}

# Run the test(s):
cd ${INTEGRATION_DIR}
set +e
go test -run ".*Log.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --log_rpc_server="localhost:${RPC_PORT}"
RESULT=$?
set -e

echo "Stopping Log RPC server on port ${RPC_PORT}"
trap - EXIT
kill -INT ${RPC_SERVER_PID}

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "Server log:"
    echo "--------------------"
    cat ${TMPDIR}/trillian_log_server.INFO
    exit $RESULT
fi
