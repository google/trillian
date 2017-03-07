#!/bin/bash
# Prepare a set of running processes for a CT test.  This script should be loaded
# with ". integration/ct_prep_test.sh", and it will populate:
#  - RPC_PORTS       : list of RPC ports (space separated)
#  - RPC_SERVERS     : list of RPC addresses (comma separated)
#  - CT_PORTS        : list of HTTP ports (space separated)
#  - CT_SERVERS      : list of HTTP addresses (comma separated)
#  - LB_PORT         : port for RPC load balancer
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LOG_SIGNER_PIDS : bash array of signer pids
#  - LB_SERVER_PID   : RPC load balancer pid
#  - CT_SERVER_PIDS  : bash array of CT HTTP server pids
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building code"
go build ${GOFLAGS} ./server/trillian_log_server/
go build ${GOFLAGS} ./server/trillian_log_signer/
go build ${GOFLAGS} ./testonly/loglb
go build ${GOFLAGS} ./examples/ct/ct_server/

yes | "${SCRIPTS_DIR}"/resetdb.sh
echo "Provisioning test log (Tree ID: 0) in database"
"${SCRIPTS_DIR}"/createlog.sh 0

# Default to one RPC server and one HTTP server.
RPC_SERVER_COUNT=${1:-1}
HTTP_SERVER_COUNT=${2:-1}
LOG_SIGNER_COUNT=1

. "${INTEGRATION_DIR}"/ct_config.sh

# Start a set of Log RPC servers.
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a RPC_SERVER_PIDS
for ((i=0; i < RPC_SERVER_COUNT; i++)); do
  port=$(pickUnusedPort)
  RPC_PORTS="${RPC_PORTS} ${port}"
  RPC_SERVERS="${RPC_SERVERS},localhost:${port}"

  echo "Starting Log RPC server on port ${port}"
  ./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --port ${port} --export_metrics=false &
  pid=$!
  RPC_SERVER_PIDS+=(${pid})
  waitForServerStartup ${port}
done
RPC_PORTS="${RPC_PORTS:1}"
RPC_SERVERS="${RPC_SERVERS:1}"

# Start a toy gRPC load balancer.  It randomly sprays RPCs across the
# backends.
LB_PORT=$(pickUnusedPort)
pushd "${TRILLIAN_ROOT}" > /dev/null
echo "Starting Log RPC load balancer ${LB_PORT} -> ${RPC_SERVERS}"
./loglb --backends ${RPC_SERVERS} --port ${LB_PORT} &
LB_SERVER_PID=$!
popd > /dev/null
waitForServerStartup ${LB_PORT}

# Start a single signer.
# TODO(drysdale): update to run multiple signers once the Trillian open-source code includes
# some kind of sequencer/signer mastership election.
declare -a LOG_SIGNER_PIDS
for ((i=0; i < LOG_SIGNER_COUNT; i++)); do
  echo "Starting Log signer"
  ./trillian_log_signer --private_key_password=towel --private_key_file=${TESTDATA}/log-rpc-server.privkey.pem --sequencer_sleep_between_runs="1s" --batch_size=500 --export_metrics=false --num_sequencers 2 &
  pid=$!
  LOG_SIGNER_PIDS+=(${pid})
done

# Start a set of CT personalities.
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a HTTP_SERVER_PIDS
for ((i=0; i < HTTP_SERVER_COUNT; i++)); do
  port=$(pickUnusedPort)
  CT_PORTS="${CT_PORTS} ${port}"
  CT_SERVERS="${CT_SERVERS},localhost:${port}"

  echo "Starting CT HTTP server on port ${port}"
  ./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${LB_PORT}" --port=${port} &
  pid=$!
  HTTP_SERVER_PIDS+=(${pid})

  set +e
  waitForServerStartup ${port}
  set -e
done
CT_PORTS="${CT_PORTS:1}"
CT_SERVERS="${CT_SERVERS:1}"
popd > /dev/null

echo "Servers running; clean up with: kill ${HTTP_SERVER_PIDS[@]} ${LB_SERVER_PID} ${RPC_SERVER_PIDS[@]}; rm ${CT_CFG}"
