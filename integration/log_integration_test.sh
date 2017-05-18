#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

echo "Launching core Trillian log components"
log_prep_test 1 1

# Cleanup for the Trillian components
TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
TO_KILL+=(${LOG_SIGNER_PIDS[@]})
TO_KILL+=(${RPC_SERVER_PIDS[@]})
TO_KILL+=(${ETCD_PID})

echo "Provision log"
go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/
TEST_TREE_ID=$(./createtree \
  --admin_server="${RPC_SERVER_1}" \
  --pem_key_path=testdata/log-rpc-server.privkey.pem \
  --pem_key_path=${GOPATH}/src/github.com/google/trillian/testdata/log-rpc-server.privkey.pem \
  --pem_key_password=towel \
  --signature_algorithm=ECDSA)
echo "Created tree ${TEST_TREE_ID}"

echo "Running test"
pushd "${INTEGRATION_DIR}"
set +e
go test -run ".*LiveLog.*" --timeout=5m ./ --treeid ${TEST_TREE_ID} --log_rpc_server="${RPC_SERVER_1}"
RESULT=$?
set -e
popd

log_stop_test
TO_KILL=()

if [ $RESULT != 0 ]; then
  sleep 1
  echo "Server log:"
  echo "--------------------"
  cat "${TMPDIR}"/trillian_log_server.INFO
  echo "Signer log:"
  echo "--------------------"
  cat "${TMPDIR}"/trillian_log_signer.INFO
  exit $RESULT
fi
