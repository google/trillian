#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

RPC_PORT=36962
CT_PORTS="6962 6963 6964"
CT_SERVERS="localhost:6962,localhost:6963,localhost:6964"

. ${INTEGRATION_DIR}/ct_config.sh

echo "Starting Log RPC server on port ${RPC_PORT}"
pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${RPC_PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
RPC_SERVER_PID=$!
popd > /dev/null

# Set an exit trap to ensure we kill the RPC server once we're done.
trap "kill -INT ${RPC_SERVER_PID}" EXIT
waitForServerStartup ${RPC_PORT}

pushd ${TRILLIAN_ROOT} > /dev/null
go build ${GOFLAGS} ./examples/ct/ct_server/
for port in ${CT_PORTS}
do
    echo "Starting CT HTTP server on port ${port}"
    ./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${RPC_PORT}" --port=${port} &
    HTTP_SERVER_PIDS="${HTTP_SERVER_PIDS} $!"
done
popd > /dev/null

# Set an exit trap to ensure we kill the servers once we're done.
trap "kill -INT ${HTTP_SERVER_PIDS} ${RPC_SERVER_PID}" EXIT
set +e
for port in ${CT_PORTS}
do
    waitForServerStartup ${port}
done
set -e

echo "Running test(s)"
set +e
go test -v -run ".*CT.*" --timeout=5m ./integration --log_config ${CT_CFG} --ct_http_server=${CT_SERVERS} --testdata=${TESTDATA}
RESULT=$?
set -e

rm ${CT_CFG}
for pid in ${HTTP_SERVER_PIDS}
do
    echo "Stopping CT HTTP server (pid ${pid})"
    kill -INT ${pid}
done
echo "Stopping Log RPC server (pid ${RPC_SERVER_PID}) on port ${RPC_PORT}"
kill -INT ${RPC_SERVER_PID}

trap - EXIT
exit $RESULT
