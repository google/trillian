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
  PKCS11_MODULE=${PKCS11_MODULE:-/usr/lib/x86_64-linux-gnu/softhsm/libsofthsm2.so}
  export SOFTHSM2_CONF=testdata/softhsm2.conf
  mkdir -p testdata/softhsm2-tokens
  echo directories.tokendir = ${PWD}/testdata/softhsm2-tokens > ${SOFTHSM2_CONF}
  softhsm2-util --module ${PKCS11_MODULE} --slot 0 --init-token --label log --pin 1234 --so-pin 5678
  softhsm2-util --module ${PKCS11_MODULE} --slot 0 --import testdata/log-rpc-server.privkey-decrypted.pem --label log_key --pin 1234 --id BEEF
  KEY_ARGS="--private_key_format PKCS11ConfigFile --pkcs11_config_path testdata/pkcs11-conf.json"
else
  KEY_ARGS="--pem_key_path=testdata/log-rpc-server.privkey.pem --pem_key_password=towel"
fi

echo "Provision log"
go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/
TEST_TREE_ID=$(./createtree \
  --admin_server="${RPC_SERVER_1}" \
  --signature_algorithm=ECDSA \
  ${KEY_ARGS})
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

if [[ "${WITH_PKCS11}" == "true" ]]; then
  rm -r testdata/softhsm2-tokens
fi

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
