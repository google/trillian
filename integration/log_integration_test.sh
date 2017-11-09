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

if [[ "${WITH_PKCS11}" == "true" ]]; then
  echo 0:${TMPDIR}/softhsm-slot0.db > ${SOFTHSM_CONF}
  softhsm --slot 0 --init-token --label log --pin 1234 --so-pin 5678
  softhsm --slot 0 --import testdata/log-rpc-server-pkcs11.privkey.pem --label log_key --pin 1234 --id BEEF
  KEY_ARGS="--private_key_format=PKCS11ConfigFile --pkcs11_config_path=testdata/pkcs11-conf.json --signature_algorithm=RSA"
else
  KEY_ARGS="--private_key_format=PrivateKey --pem_key_path=testdata/log-rpc-server.privkey.pem --pem_key_password=towel --signature_algorithm=ECDSA"
fi

echo "Provision log"
go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/
TEST_TREE_ID=$(./createtree \
  --admin_server="${RPC_SERVER_1}" \
  ${KEY_ARGS})
echo "Created tree ${TEST_TREE_ID}"

echo "Running test"
pushd "${INTEGRATION_DIR}"
set +e
TRILLIAN_SQL_DRIVER=mysql go test ${GOFLAGS} \
  -run ".*LiveLog.*" \
  -timeout=${GO_TEST_TIMEOUT:-5m} \
  ./ --log_rpc_server="${RPC_SERVER_1}" --treeid ${TEST_TREE_ID}
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
