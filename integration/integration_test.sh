#!/bin/bash
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

runTest map_integration_test.sh "Map integration test"

