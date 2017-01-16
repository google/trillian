#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

RPC_PORTS="36962 36963 36964"
RPC_SERVERS="localhost:36962,localhost:36963,localhost:36964"
LB_PORT=46962
CT_PORTS="6962 6963 6964"
CT_SERVERS="localhost:6962,localhost:6963,localhost:6964"

. "${INTEGRATION_DIR}"/ct_config.sh

# Start a set of Log RPC servers.  Note that each of them will run their own
# sequencer; a proper deployment should have a single master sequencer, but
# for this test we rely on the transactional nature of the sequencing operation.
# TODO(drysdale): update this comment once the Trillian open-source code includes
# some kind of sequencer mastership election.
pushd "${TRILLIAN_ROOT}" > /dev/null
go build ${GOFLAGS} ./server/trillian_log_server/
for port in ${RPC_PORTS}
do
    echo "Starting Log RPC server on port ${port}"
    ./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${port} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 --export_metrics=false &
    RPC_SERVER_PIDS="${RPC_SERVER_PIDS} $!"
done
popd > /dev/null

# Set an exit trap to ensure we kill the RPC servers once we're done.
trap "kill -INT ${RPC_SERVER_PIDS}" EXIT
for port in ${RPC_PORTS}
do
    waitForServerStartup ${port}
done

# Start a toy gRPC load balancer.  It randomly sprays RPCs across the
# backends.
pushd "${TRILLIAN_ROOT}" > /dev/null
go build ${GOFLAGS} ./testonly/loglb
echo "Starting Log RPC load balancer ${LB_PORT} -> ${RPC_SERVERS}"
./loglb --backends ${RPC_SERVERS} --port ${LB_PORT} &
LB_SERVER_PID=$!
popd > /dev/null
trap "kill -INT ${LB_SERVER_PID} ${RPC_SERVER_PIDS}" EXIT
waitForServerStartup ${LB_PORT}


# Start a set of CT personalities.
pushd "${TRILLIAN_ROOT}" > /dev/null
go build ${GOFLAGS} ./examples/ct/ct_server/
for port in ${CT_PORTS}
do
    echo "Starting CT HTTP server on port ${port}"
    ./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${LB_PORT}" --port=${port} &
    HTTP_SERVER_PIDS="${HTTP_SERVER_PIDS} $!"
done
popd > /dev/null

# Set an exit trap to ensure we kill the servers once we're done.
trap "kill -INT ${HTTP_SERVER_PIDS} ${LB_SERVER_PID} ${RPC_SERVER_PIDS}" EXIT
set +e
for port in ${CT_PORTS}
do
    waitForServerStartup ${port}
done
set -e

echo "Running test(s)"
set +e
go test -v -run ".*CT.*" --timeout=5m ./integration --log_config "${CT_CFG}" --ct_http_server=${CT_SERVERS} --testdata=${TESTDATA}
RESULT=$?
set -e

rm "${CT_CFG}"
for pid in ${HTTP_SERVER_PIDS}
do
    echo "Stopping CT HTTP server (pid ${pid})"
    kill -INT ${pid}
done
echo "Stopping Log RPC load balancer (pid ${LB_SERVER_PID})"
kill -INT ${LB_SERVER_PID}
for pid in ${RPC_SERVER_PIDS}
do
    echo "Stopping Log RPC server (pid ${pid})"
    kill -INT ${pid}
done

trap - EXIT
exit $RESULT
