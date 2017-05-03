#!/bin/bash
# Close down a set of running processes for a CT test. Assumes the following
# variables are set, in addition to those needed by log_stop_test.sh:
#  - CT_SERVER_PIDS  : bash array of CT HTTP server pids
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

for pid in "${CT_SERVER_PIDS[@]}"; do
  echo "Stopping CT HTTP server (pid ${pid})"
  killPid ${pid}
done

. "${INTEGRATION_DIR}"/log_stop_test.sh
