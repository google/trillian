#!/bin/bash
# Prepare a set of running processes for a CT test.
# This script should be loaded # with ". integration/ct_prep_test.sh";
# it will populate:
#  - CT_SERVERS     : list of HTTP addresses (comma separated)
#  - CT_SERVER_PIDS : bash array of CT HTTP server pids
# in addition to the variables populated by log_prep_test.sh.

set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

# Default to one of everything.
RPC_SERVER_COUNT=${1:-1}
LOG_SIGNER_COUNT=${2:-1}
HTTP_SERVER_COUNT=${3:-1}

echo "Launching core Trillian log components"
. "${INTEGRATION_DIR}"/log_prep_test.sh "${RPC_SERVER_COUNT}" "${LOG_SIGNER_COUNT}"

echo "Building CT personality code"
go build ${GOFLAGS} ./examples/ct/ct_server/

echo "Provisioning logs for CT"
. "${INTEGRATION_DIR}"/ct_config.sh "${ADMIN_SERVER}"

echo "Launching CT personalities"
pushd "${TRILLIAN_ROOT}" > /dev/null
declare -a CT_SERVER_PIDS
for ((i=0; i < HTTP_SERVER_COUNT; i++)); do
  port=$(pickUnusedPort)
  CT_SERVERS="${CT_SERVERS},localhost:${port}"

  echo "Starting CT HTTP server on localhost:${port}"
  ./ct_server --log_config=${CT_CFG} --log_rpc_server="localhost:${LB_PORT}" --port=${port} &
  pid=$!
  CT_SERVER_PIDS+=(${pid})

  set +e
  waitForServerStartup ${port}
  set -e
done
CT_SERVERS="${CT_SERVERS:1}"
popd > /dev/null

echo "Servers running; clean up with: kill ${CT_SERVER_PIDS[@]} ${LB_SERVER_PID} ${RPC_SERVER_PIDS[@]} ${LOG_SIGNER_PIDS[@]} ${ETCD_PID}; rm -rf ${CT_CFG} ${ETCD_DB_DIR}"
