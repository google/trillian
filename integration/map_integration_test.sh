#!/bin/bash
set -e

INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
TRILLIAN_ROOT=${INTEGRATION_DIR}/..
TESTDATA=${TRILLIAN_ROOT}/testdata

TEST_TREE_ID=123
PORT=34556

echo "=== RUN   Map Integration Test"
echo "Provisioning test map (Tree ID: $TEST_TREE_ID) in database"

mysql -u test --password=zaphod -D test -e "DELETE FROM Trees WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "INSERT INTO Trees VALUES (${TEST_TREE_ID}, 1, 'TESTMAP', 'SHA256', 'SHA256', false)"

echo "Starting Map sever on port ${PORT}"

go build ./server/vmap/trillian_map_server/
./trillian_map_server --private_key_password=towel --private_key_file=${TESTDATA}/trillian-map-server-key.pem --port ${PORT} &
sleep 2

echo "Starting integration test"

cd ${INTEGRATION_DIR}
go test -tags=integration ./ --map_id ${TEST_TREE_ID} --server="localhost:${PORT}"

echo "Stopping Map server on port ${PORT}"
echo "--- PASS: Map Integration Test"

kill -INT %1
