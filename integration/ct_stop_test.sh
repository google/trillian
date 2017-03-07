#!/bin/bash
# Close down a set of running processes for a CT test. Assumes the following
# variables are set:
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LB_SERVER_PID   : RPC load balancer pid
#  - CT_SERVER_PIDS  : bash array of CT HTTP server pids
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

for pid in "${HTTP_SERVER_PIDS[@]}"; do
    echo "Stopping CT HTTP server (pid ${pid})"
    killPid ${pid}
done
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
