#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

echo "=== RUN   Integration Test Suite"
${INTEGRATION_DIR}/map_integration_test.sh
echo "--- PASS: Integration Test Suite"

