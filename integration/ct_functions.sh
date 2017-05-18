# Functions for setting up CT personalities in Trillian integration tests
# Requires github.com/google/trillian/integration/functions.sh

declare -a CT_SERVER_PIDS
CT_SERVERS=
CT_CFG=

# ct_prep_test prepares a set of running processes for a CT test.
# Parameters:
#   - number of log servers to run
#   - number of log signers to run
#   - number of CT personality instances to run
# Populates:
#  - CT_SERVERS     : list of HTTP addresses (comma separated)
#  - CT_SERVER_PIDS : bash array of CT HTTP server pids
# in addition to the variables populated by logPrepTest.
ct_prep_test() {
  # Default to one of everything.
  local rpc_server_count=${1:-1}
  local log_signer_count=${2:-1}
  local http_server_count=${3:-1}

  echo "Launching core Trillian log components"
  log_prep_test "${rpc_server_count}" "${log_signer_count}"

  echo "Building CT personality code"
  go build ${GOFLAGS} github.com/google/trillian/examples/ct/ct_server

  echo "Provisioning logs for CT"
  ct_provision "${RPC_SERVER_1}"

  echo "Launching CT personalities"
  for ((i=0; i < http_server_count; i++)); do
    port=$(pick_unused_port)
    CT_SERVERS="${CT_SERVERS},localhost:${port}"

    echo "Starting CT HTTP server on localhost:${port}"
    ./ct_server ${ETCD_OPTS} --log_config="${CT_CFG}" --log_rpc_server="${RPC_SERVERS}" --port=${port} &
    pid=$!
    CT_SERVER_PIDS+=(${pid})
    wait_for_server_startup ${port}
  done
  CT_SERVERS="${CT_SERVERS:1}"
}

# ct_provision generates a CT configuration file and provisions the trees for it.
# Parameters:
#   - location of admin server instance
# Populates:
#   - CT_CFG : configuration file for CT personality
ct_provision() {
  local admin_server="$1"

  # Build config file with absolute paths
  CT_CFG=$(mktemp ${TMPDIR}/ct-XXXXXX)

  sed "s!@TESTDATA@!${GOPATH}/src/github.com/google/trillian/testdata!" ${GOPATH}/src/github.com/google/trillian/integration/ct_integration_test.cfg > "${CT_CFG}"

  echo 'Building createtree'
  go build ${GOFLAGS} github.com/google/trillian/cmd/createtree/

  num_logs=$(grep -c '@TREE_ID@' "${CT_CFG}")
  for i in $(seq ${num_logs}); do
    # TODO(daviddrysdale): Consider using distinct keys for each log
    tree_id=$(./createtree \
      --admin_server="${admin_server}" \
      --pem_key_path=${GOPATH}/src/github.com/google/trillian/testdata/log-rpc-server.privkey.pem \
      --pem_key_password=towel \
      --signature_algorithm=ECDSA)
    echo "Created tree ${tree_id}"
    # Need suffix for sed -i to cope with both GNU and non-GNU (e.g. OS X) sed.
    sed -i'.bak' "1,/@TREE_ID@/s/@TREE_ID@/${tree_id}/" "${CT_CFG}"
    rm -f "${CT_CFG}.bak"
  done

  echo "CT configuration:"
  cat "${CT_CFG}"
  echo
}

# ct_stop_test closes the running processes for a CT tests.
# Assumes the following variables are set, in addition to those needed by logStopTest:
#  - CT_SERVER_PIDS  : bash array of CT HTTP server pids
ct_stop_test() {
  for pid in "${CT_SERVER_PIDS[@]}"; do
    echo "Stopping CT HTTP server (pid ${pid})"
    kill_pid ${pid}
  done
  log_stop_test
}
