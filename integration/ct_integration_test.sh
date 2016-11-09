#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

TEST_TREE_ID=6962
RPC_PORT=36962
CT_PORT=6962

echo "Provisioning test log (Tree ID: $TEST_TREE_ID) in database"
wipeLog ${TEST_TREE_ID}
createLog ${TEST_TREE_ID}

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the RPC server once we're done.
trap "kill -INT ${RPC_SERVER_PID}" EXIT
sleep 2

echo "Starting CT HTTP server on port ${CT_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./examples/ct/ct_server/
# TODO(drysdale): drop the --log_id flag once the CT HTTP server is multi-tenant
./ct_server --private_key_password=dirk --private_key_file=${TESTDATA}/ct-http-server.privkey.pem --public_key_file=${TESTDATA}/ct-http-server.pubkey.pem --log_rpc_server="localhost:${RPC_PORT}" --port=${CT_PORT} --trusted_roots=${TESTDATA}/fake-ca.cert --log_id ${TEST_TREE_ID} &
HTTP_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the servers once we're done.
trap "kill -INT ${HTTP_SERVER_PID} ${RPC_SERVER_PID}" EXIT
sleep 2

echo "Running test(s)"
cd ${INTEGRATION_DIR}
set +e
go test -tags=integration -run ".*CT.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --ct_http_server="localhost:${CT_PORT}" --public_key_file=${TESTDATA}/ct-http-server.pubkey.pem
RESULT=$?
set -e

trap - EXIT
echo "Stopping CT HTTP server (pid ${HTTP_SERVER_PID}) on port ${CT_PORT}"
kill -INT ${HTTP_SERVER_PID}

echo "Stopping Log RPC server (pid ${RPC_SERVER_PID}) on port ${RPC_PORT}"
kill -INT ${RPC_SERVER_PID}

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "RPC Server log:"
    echo "--------------------"
    cat ${TMPDIR}/trillian_log_server.INFO
    echo "HTTP Server log:"
    echo "--------------------"
    cat ${TMPDIR}/ct_server.INFO
fi
