#!/bin/bash
set -e

INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

runTest "Map integration test" map_integration_test.sh
runTest "Log integration test" log_integration_test.sh
runTest "CT integration test" ct_integration_test.sh 1 1
runTest "CT multi-server integration test" ct_integration_test.sh 3 3
