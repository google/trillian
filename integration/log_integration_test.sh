#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./cmd/createtree/
go build ${GOFLAGS} ./server/trillian_log_server/
go build ${GOFLAGS} ./server/trillian_log_signer/

yes | "${SCRIPTS_DIR}"/resetdb.sh

RPC_PORT=$(pickUnusedPort)

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_log_server --port ${RPC_PORT} &
RPC_SERVER_PID=$!
popd > /dev/null
waitForServerStartup ${RPC_PORT}

TEST_TREE_ID=$(./createtree --admin_endpoint="localhost:${RPC_PORT}" --pem_key_path=testdata/log-rpc-server.privkey.pem --pem_key_password=towel)
echo "Created tree ${TEST_TREE_ID}"

# Ensure we kill the RPC server once we're done.
TO_KILL+=(${RPC_SERVER_PID})
waitForServerStartup ${RPC_PORT}

echo "Starting Log signer"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_log_signer --sequencer_sleep_between_runs="1s" --batch_size=100 --export_metrics=false &
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
