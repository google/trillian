#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

TEST_TREE_ID=123
PORT=34556

echo "Provisioning test map (Tree ID: $TEST_TREE_ID) in database"

mysql -u test --password=zaphod -D test -e "DELETE FROM Trees WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "INSERT INTO Trees VALUES (${TEST_TREE_ID}, 1, 'MAP', 'SHA256', 'SHA256', false)"

echo "Starting Map server on port ${PORT}"

# Start the map server, and set an exit trap to ensure we kill it
# once we're done:
pushd ${TRILLIAN_ROOT} > /dev/null
go build ./server/vmap/trillian_map_server/
./trillian_map_server --private_key_password=towel --private_key_file=${TESTDATA}/trillian-map-server-key.pem --port ${PORT} &
trap "kill -INT %1" EXIT
popd > /dev/null
sleep 2


# Run the test(s):
cd ${INTEGRATION_DIR}
go test -tags=integration -run ".*Map.*" --timeout=5m ./ --map_id ${TEST_TREE_ID} --server="localhost:${PORT}"

echo "Stopping Map server on port ${PORT}"
