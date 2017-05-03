#!/bin/bash
# Close down a set of running processes for a CT test. Assumes the following
# variables are set:
#  - LOG_SIGNER_PIDS : bash array of signer pids
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LB_SERVER_PID   : RPC load balancer pid
#  - ETCD_PID        : etcd pid
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

for pid in "${LOG_SIGNER_PIDS[@]}"; do
  echo "Stopping Log signer (pid ${pid})"
  killPid ${pid}
done
echo "Stopping Log RPC load balancer (pid ${LB_SERVER_PID})"
killPid ${LB_SERVER_PID}
for pid in "${RPC_SERVER_PIDS[@]}"; do
  echo "Stopping Log RPC server (pid ${pid})"
  killPid ${pid}
done
if [[ "${ETCD_PID}" != "" ]]; then
  echo "Stopping local etcd server (pid ${ETCD_PID})"
  killPid ${ETCD_PID}
fi
