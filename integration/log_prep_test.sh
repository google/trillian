#!/bin/bash
# Prepare a set of running processes for a Trillian log test.
# This script should be loaded with ". integration/log_prep_test.sh",
# and it will populate:
#  - RPC_SERVERS     : list of RPC addresses (comma separated)
#  - LB_PORT         : port for RPC load balancer
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LOG_SIGNER_PIDS : bash array of signer pids
#  - LB_SERVER_PID   : RPC load balancer pid
#  - ETCD_PID        : etcd pid
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

echo "Building Trillian log code"
go build ${GOFLAGS} ./server/trillian_log_server/
go build ${GOFLAGS} ./server/trillian_log_signer/
go build ${GOFLAGS} ./testonly/loglb

yes | "${SCRIPTS_DIR}"/resetdb.sh

# Default to one of each.
RPC_SERVER_COUNT=${1:-1}
LOG_SIGNER_COUNT=${2:-1}

# Start a local etcd instance (if configured).
if [[ -x "${ETCD_DIR}/etcd" ]]; then
  ETCD_PORT=2379
  ETCD_SERVER="localhost:${ETCD_PORT}"
  echo "Starting local etcd server on ${ETCD_SERVER}"
  ${ETCD_DIR}/etcd &
  ETCD_PID=$!
  ETCD_DB_DIR=default.etcd
  set +e
  waitForServerStartup ${ETCD_PORT}
  set -e
  SIGNER_ELECTION_OPTS="--etcd_servers=${ETCD_SERVER}"
else
  if  [[ ${LOG_SIGNER_COUNT} > 1 ]]; then
    echo "*** Warning: running multiple signers with no etcd instance ***"
  fi
  SIGNER_ELECTION_OPTS="--force_master"
fi

# Start a set of Log RPC servers.
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a RPC_SERVER_PIDS
for ((i=0; i < RPC_SERVER_COUNT; i++)); do
  port=$(pickUnusedPort)
  RPC_SERVERS="${RPC_SERVERS},localhost:${port}"

  echo "Starting Log RPC server on localhost:${port}"
  ./trillian_log_server --rpc_endpoint="localhost:${port}" --http_endpoint='' &
  pid=$!
  RPC_SERVER_PIDS+=(${pid})
  waitForServerStartup ${port}

  # Use the first Log server as the Admin server (any would do)
  if [[ $i -eq 0 ]]; then
    ADMIN_SERVER="localhost:$port"
  fi
done
RPC_SERVERS="${RPC_SERVERS:1}"
popd > /dev/null

# Start a toy gRPC load balancer.  It randomly sprays RPCs across the
# backends.
LB_PORT=$(pickUnusedPort)
pushd "${TRILLIAN_ROOT}" > /dev/null
echo "Starting Log RPC load balancer ${LB_PORT} -> ${RPC_SERVERS}"
./loglb --backends ${RPC_SERVERS} --port ${LB_PORT} &
LB_SERVER_PID=$!
popd > /dev/null
waitForServerStartup ${LB_PORT}

# Start a set of signers.
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a LOG_SIGNER_PIDS
for ((i=0; i < LOG_SIGNER_COUNT; i++)); do
  echo "Starting Log signer"
  ./trillian_log_signer "${SIGNER_ELECTION_OPTS}" --sequencer_interval="1s" --batch_size=500 --http_endpoint='' --num_sequencers 2 &
  pid=$!
  LOG_SIGNER_PIDS+=(${pid})
done

echo "Servers running; clean up with: kill ${LB_SERVER_PID} ${RPC_SERVER_PIDS[@]} ${LOG_SIGNER_PIDS[@]} ${ETCD_PID}; rm -rf ${ETCD_DB_DIR}"
