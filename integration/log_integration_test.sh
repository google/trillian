#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
. ${INTEGRATION_DIR}/common.sh

TEST_TREE_ID=1123
PORT=34557

echo "Provisioning test log (Tree ID: $TEST_TREE_ID) in database"

mysql -u test --password=zaphod -D test -e "DELETE FROM Unsequenced WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "DELETE FROM TreeHead WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "DELETE FROM SequencedLeafData WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "DELETE FROM LeafData WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "DELETE FROM Trees WHERE TreeId = ${TEST_TREE_ID}"
mysql -u test --password=zaphod -D test -e "INSERT INTO Trees VALUES (${TEST_TREE_ID}, 1, 'LOG', 'SHA256', 'SHA256', false)"

echo "Starting Log server on port ${PORT}"

# Start the log server, and set an exit trap to ensure we kill it once we're done:
pushd ${TRILLIAN_ROOT} > /dev/null
go build ./server/trillian_log_server/
./trillian_log_server --private_key_password=towel --private_key_file=${TESTDATA}/trillian-server-key.pem --port ${PORT} --signer_interval="1s" --sequencer_sleep_between_runs="1s" --batch_size=100 &
trap "kill -INT %1" EXIT
popd > /dev/null
sleep 2


# Run the test(s):
cd ${INTEGRATION_DIR}
go test -tags=integration -run ".*Log.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --log_server="localhost:${PORT}"

echo "Stopping Log server on port ${PORT}"
