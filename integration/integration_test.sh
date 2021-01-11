#!/bin/bash
set -e

INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

run_test "Map integration test" "${INTEGRATION_DIR}/map_integration_test.sh" "$@"
run_test "Log integration test" "${INTEGRATION_DIR}/log_integration_test.sh" "$@"