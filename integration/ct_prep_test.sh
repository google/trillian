#!/bin/bash
# Prepare a set of running processes for a CT test.  This script should be loaded
# with ". integration/ct_prep_test.sh", and it will populate:
#  - RPC_PORTS       : list of RPC ports (space separated)
#  - RPC_SERVERS     : list of RPC addresses (comma separated)
#  - CT_PORTS        : list of HTTP ports (space separated)
#  - CT_SERVERS      : list of HTTP addresses (comma separated)
#  - LB_PORT         : port for RPC load balancer
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LB_SERVER_PID   : RPC load balancer pid
#  - CT_SERVER_PIDS  : bash array of CT HTTP server pids
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./server/trillian_log_server/
go build ${GOFLAGS} ./testonly/loglb
go build ${GOFLAGS} ./examples/ct/ct_server/

yes | "${SCRIPTS_DIR}"/resetdb.sh
echo "Provisioning test log (Tree ID: 0) in database"
"${SCRIPTS_DIR}"/createlog.sh 0

# Default to one RPC server and one HTTP server.
RPC_SERVER_COUNT=${1:-1}
HTTP_SERVER_COUNT=${2:-1}
BASE_RPC_PORT=36961
BASE_HTTP_PORT=6961
LB_PORT=46962

port=${BASE_RPC_PORT}
for ((i=0; i < RPC_SERVER_COUNT; i++)); do
  port=$((port + 2))
  if [[ $i -eq 0 ]]; then
    RPC_PORTS="${port}"
    RPC_SERVERS="localhost:${port}"
  else
    RPC_PORTS="${RPC_PORTS} ${port}"
    RPC_SERVERS="${RPC_SERVERS},localhost:${port}"
  fi
done

port=${BASE_HTTP_PORT}
for ((i=0; i < HTTP_SERVER_COUNT; i++)); do
  port=$((port + 1))
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
declare -a RPC_SERVER_PIDS
for port in ${RPC_PORTS}; do
    echo "Starting Log RPC server on port ${port}"
    ./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${port} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 --export_metrics=false &
    pid=$!
    RPC_SERVER_PIDS+=(${pid})
done
popd > /dev/null

for port in ${RPC_PORTS}; do
    waitForServerStartup ${port}
done

# Start a toy gRPC load balancer.  It randomly sprays RPCs across the
# backends.
pushd "${TRILLIAN_ROOT}" > /dev/null
echo "Starting Log RPC load balancer ${LB_PORT} -> ${RPC_SERVERS}"
./loglb --backends ${RPC_SERVERS} --port ${LB_PORT} &
LB_SERVER_PID=$!
popd > /dev/null
waitForServerStartup ${LB_PORT}


# Start a set of CT personalities.
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a HTTP_SERVER_PIDS
for port in ${CT_PORTS}; do
    echo "Starting CT HTTP server on port ${port}"
    ./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${LB_PORT}" --port=${port} &
    pid=$!
    HTTP_SERVER_PIDS+=(${pid})
done
popd > /dev/null

set +e
for port in ${CT_PORTS}; do
    waitForServerStartup ${port}
done
set -e

echo "Servers running; clean up with: kill ${HTTP_SERVER_PIDS[@]} ${LB_SERVER_PID} ${RPC_SERVER_PIDS[@]}; rm ${CT_CFG}"
