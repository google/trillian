#!/bin/bash
#
# run_integration_test.sh is a wrapper around the log_prep and run_test functions.
# It's intended to be used by the Travis CT/Trillian integration test only,
# and will go away when we migrate off of Travis.
set -e

INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh


echo "Launching core Trillian log components"
log_prep_test 1 1

# Cleanup for the Trillian components
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})

export TRILLIAN_LOG_SERVERS="${RPC_SERVERS}"
export TRILLIAN_LOG_SERVER_1="${RPC_SERVER_1}"

echo "Running test"
run_test "$1" "$2"
RESULT=$?

log_stop_test
TO_KILL=()

if [ $RESULT != 0 ]; then
    sleep 1
    echo "Server log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_server.INFO
    echo "Signer log:"
    echo "--------------------"
    cat "${TMPDIR}"/trillian_log_signer.INFO
fi

exit $RESULT
