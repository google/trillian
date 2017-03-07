#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./server/vmap/trillian_map_server/

TEST_TREE_ID=123
RPC_PORT=$(pickUnusedPort)

echo "Provisioning test map (Tree ID: $TEST_TREE_ID) in database"
yes | "${SCRIPTS_DIR}"/resetdb.sh
"${SCRIPTS_DIR}"/createmap.sh ${TEST_TREE_ID}

echo "Starting Map RPC server on port ${RPC_PORT}"
pushd "${TRILLIAN_ROOT}" > /dev/null
./trillian_map_server --private_key_password=towel --private_key_file=${TESTDATA}/map-rpc-server.privkey.pem --port ${RPC_PORT} &
RPC_SERVER_PID=$!
popd > /dev/null

# Ensure we kill the RPC server once we're done.
TO_KILL+=(${RPC_SERVER_PID})
waitForServerStartup ${RPC_PORT}

# Run the test(s):
cd "${INTEGRATION_DIR}"
set +e
go test -run ".*LiveMap.*" --timeout=5m ./ --map_id ${TEST_TREE_ID} --map_rpc_server="localhost:${RPC_PORT}"
RESULT=$?
set -e

echo "Stopping MAP RPC server (pid ${RPC_SERVER_PID})"
killPid ${RPC_SERVER_PID}
TO_KILL=()

if [ $RESULT != 0 ]; then
    sleep 1
    if [ "$TMPDIR" == "" ]; then
        TMPDIR=/tmp
    fi
    echo "Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_map_server.INFO
    exit $RESULT
fi
