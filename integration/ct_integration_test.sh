#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

RPC_PORT=36962
CT_PORT=6962

. "${INTEGRATION_DIR}"/ct_config.sh

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Ensure we kill the RPC server once we're done.
TO_KILL="${RPC_SERVER_PID}"
waitForServerStartup ${RPC_PORT}

echo "Starting CT HTTP server on port ${CT_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
go build ${GOFLAGS} ./examples/ct/ct_server/
./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${RPC_PORT}" --port=${CT_PORT} &
HTTP_SERVER_PID=$!
popd > /dev/null

# Ensure we kill the servers once we're done.
TO_KILL="${HTTP_SERVER_PID} ${RPC_SERVER_PID}"

set +e
waitForServerStartup ${CT_PORT}
set -e

echo "Running test(s)"
set +e
go test -v -run ".*CT.*" --timeout=5m ./integration --log_config "${CT_CFG}" --ct_http_servers="localhost:${CT_PORT}" --testdata=${TESTDATA}
RESULT=$?
set -e

echo "Stopping CT HTTP server (pid ${HTTP_SERVER_PID}) on port ${CT_PORT}"
kill -INT ${HTTP_SERVER_PID}
echo "Stopping Log RPC server (pid ${RPC_SERVER_PID}) on port ${RPC_PORT}"
kill -INT ${RPC_SERVER_PID}
TO_KILL=""

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "${TMPDIR}" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "RPC Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_server.INFO
    echo "HTTP Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/ct_server.INFO
    exit $RESULT
fi
