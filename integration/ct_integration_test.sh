#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

yes | "${SCRIPTS_DIR}"/resetdb.sh
echo "Provisioning test log (Tree ID: 0) in database"
"${SCRIPTS_DIR}"/createlog.sh 0

# Default to one RPC server and one HTTP server.
RPC_SERVER_COUNT=${1:-1}
HTTP_SERVER_COUNT=${2:-1}
BASE_RPC_PORT=36961
BASE_HTTP_PORT=6961
LB_PORT=46962

for ((i=0; i < RPC_SERVER_COUNT; i++))
do
  port=$((BASE_RPC_PORT + i))
  if [[ $i -eq 0 ]]; then
    RPC_PORTS="${port}"
    RPC_SERVERS="localhost:${port}"
  else
    RPC_PORTS="${RPC_PORTS} ${port}"
    RPC_SERVERS="${RPC_SERVERS},localhost:${port}"
  fi
done

for ((i=0; i < HTTP_SERVER_COUNT; i++))
do
  port=$((BASE_HTTP_PORT + i))
  if [[ $i -eq 0 ]]; then
    CT_PORTS="${port}"
    CT_SERVERS="localhost:${port}"
  else
    CT_PORTS="${CT_PORTS} ${port}"
    CT_SERVERS="${CT_SERVERS},localhost:${port}"
  fi
done

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

# Ensure we kill the RPC servers once we're done.
TO_KILL="${RPC_SERVER_PIDS}"
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
TO_KILL="${LB_SERVER_PID} ${RPC_SERVER_PIDS}"
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

# Ensure we kill the servers once we're done.
TO_KILL="${HTTP_SERVER_PIDS} ${LB_SERVER_PID} ${RPC_SERVER_PIDS}"
set +e
for port in ${CT_PORTS}
do
    waitForServerStartup ${port}
done
set -e

echo "Running test(s)"
set +e
go test -v -run ".*CT.*" --timeout=5m ./integration --log_config "${CT_CFG}" --ct_http_servers=${CT_SERVERS} --testdata=${TESTDATA}
RESULT=$?
set -e

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
TO_KILL=""

exit $RESULT
