#!/bin/bash
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

runTest map_integration_test.sh "Map integration test"
runTest log_integration_test.sh "Log integration test"
runTest ct_integration_test.sh "CT integration test"
runTest ct_multi_test.sh "CT multi-server integration test"
