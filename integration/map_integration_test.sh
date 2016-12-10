#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

TEST_TREE_ID=123
RPC_PORT=34556

echo "Provisioning test map (Tree ID: $TEST_TREE_ID) in database"
${SCRIPTS_DIR}/wipemap.sh ${TEST_TREE_ID}
${SCRIPTS_DIR}/createmap.sh ${TEST_TREE_ID}

echo "Starting Map RPC server on port ${RPC_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./server/vmap/trillian_map_server/
./trillian_map_server --private_key_password=towel --private_key_file=${TESTDATA}/map-rpc-server.privkey.pem --port ${RPC_PORT} &
RPC_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the RPC server once we're done.
trap "kill -INT ${RPC_SERVER_PID}" EXIT
waitForServerStartup ${RPC_PORT}

# Run the test(s):
cd ${INTEGRATION_DIR}
set +e
go test -tags=integration -run ".*Map.*" --timeout=5m ./ --map_id ${TEST_TREE_ID} --map_rpc_server="localhost:${RPC_PORT}"
RESULT=$?
set -e

echo "Stopping Map RPC server on port ${RPC_PORT}"
trap - EXIT
kill -INT ${RPC_SERVER_PID}

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "Server log:"
    echo "--------------------"
    cat ${TMPDIR}/trillian_map_server.INFO
    exit $RESULT
fi
