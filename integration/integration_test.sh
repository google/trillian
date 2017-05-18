#!/bin/bash
set -e

INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

run_test "Map integration test" "${INTEGRATION_DIR}/map_integration_test.sh"
run_test "Log integration test" "${INTEGRATION_DIR}/log_integration_test.sh"
run_test "CT integration test" "${INTEGRATION_DIR}/ct_integration_test.sh" 1 1 1
run_test "CT multi-server integration test" "${INTEGRATION_DIR}/ct_integration_test.sh" 3 3 3
