#!/bin/bash
set -e
INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
. "${INTEGRATION_DIR}"/functions.sh

TRILLIAN_SERVER="$1"
TEST_STARTED_TRILLIAN_SERVER=false

if [ -z "${TRILLIAN_SERVER}" ]; then
  echo "Launching core Trillian log components"
  log_prep_test 1 1

  # Cleanup for the Trillian components
  TO_DELETE="${TO_DELETE} ${ETCD_DB_DIR}"
  TO_KILL+=(${LOG_SIGNER_PIDS[@]})
  TO_KILL+=(${RPC_SERVER_PIDS[@]})
  TO_KILL+=(${ETCD_PID})

  TRILLIAN_SERVER="${RPC_SERVER_1}"
  TEST_STARTED_TRILLIAN_SERVER=true
fi

if [[ "${WITH_PKCS11}" == "true" ]]; then
  echo 0:${TMPDIR}/softhsm-slot0.db > ${SOFTHSM_CONF}
  softhsm --slot 0 --init-token --label log --pin 1234 --so-pin 5678
  softhsm --slot 0 --import testdata/log-rpc-server-pkcs11.privkey.pem --label log_key --pin 1234 --id BEEF
  KEY_ARGS="--private_key_format=PKCS11ConfigFile --pkcs11_config_path=testdata/pkcs11-conf.json --signature_algorithm=RSA"
else
  KEY_ARGS="--private_key_format=PrivateKey --pem_key_path=testdata/log-rpc-server.privkey.pem --pem_key_password=towel --signature_algorithm=ECDSA"
fi

echo "Provision log"
go build github.com/google/trillian/cmd/createtree/
TEST_TREE_ID=$(./createtree \
  --admin_server="${TRILLIAN_SERVER}" \
  ${KEY_ARGS})
echo "Created tree ${TEST_TREE_ID}"

echo "Running test"
pushd "${INTEGRATION_DIR}"
set +e
go test \
  -run ".*LiveLog.*" \
  -timeout=${GO_TEST_TIMEOUT:-5m} \
  ./ \
  --log_rpc_server="${TRILLIAN_SERVER}" \
  --treeid ${TEST_TREE_ID} \
  --alsologtostderr
RESULT=$?
set -e
popd

if ${TEST_STARTED_TRILLIAN_SERVER}; then
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
  fi
fi

exit $RESULT
