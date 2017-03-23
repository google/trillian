#!/bin/bash
# Builds configuration for a CT test, set in ${CT_CFG} and provisions logs for
# them.
set -e

if [[ $# -ne 1 ]]; then
  echo "Usage: $0 <admin_server_address>"
  exit 1
fi
ADMIN_SERVER="$1"

INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/common.sh

# Build config file with absolute paths
CT_CFG=$(mktemp "${INTEGRATION_DIR}"/ct-XXXXXX)
sed "s!@TESTDATA@!${TESTDATA}!" ./integration/ct_integration_test.cfg > "${CT_CFG}"

echo 'Building createtree'
go build ${GOFLAGS} ./cmd/createtree/

num_logs=$(grep -c '@TREE_ID@' "${CT_CFG}")
for i in $(seq ${num_logs}); do
  # TODO(daviddrysdale): Consider using distinct keys for each log
  tree_id=$(./createtree --admin_server="${ADMIN_SERVER}" --pem_key_path=testdata/log-rpc-server.privkey.pem --pem_key_password=towel)
  echo "Created tree ${tree_id}"
  sed -i "0,/@TREE_ID@/s/@TREE_ID@/${tree_id}/" "${CT_CFG}"
done

echo "CT configuration:"
cat "${CT_CFG}"
echo
