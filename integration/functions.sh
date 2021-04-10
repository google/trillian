# Functions for setting up Trillian integration tests

if [[ -z "${TMPDIR}" ]]; then
  TMPDIR=/tmp
fi
readonly TMPDIR
declare -a RPC_SERVER_PIDS
declare -a LOG_SIGNER_PIDS
declare -a TO_KILL
declare -a TO_DELETE
HTTP_SERVER_1=''
RPC_SERVER_1=''
RPC_SERVERS=''
ETCD_OPTS=''
ETCD_PID=''
ETCD_DB_DIR=''
readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

# run_test runs the given test with additional output messages.
run_test() {
  local name=$1
  shift
  echo "=== RUN   ${name}"
  "$@"
  rc=$?
  if [ $rc -ne 0 ]; then
    echo "--- FAIL: ${name}"
  else
    echo "--- PASS: ${name}"
  fi
  return $rc
}

# wait_for_server_startup pauses until there is a response on the given port.
wait_for_server_startup() {
  # The server will 404 the request as there's no handler for it. This error doesn't matter
  # as the test will fail if the server is really not up.
  local port=$1
  set +e
  wget -q --spider --retry-connrefused --waitretry=1 -t 10 localhost:${port}
  # Wait a bit more to give it a chance to become actually available, e.g. if CI
  # environment is slow.
  sleep 2
  wget -q --spider -t 1 localhost:${port}
  local rc=$?
  set -e
  # wget emits rc=8 for server issuing an error response (e.g. 404)
  if [ ${rc} != 0 -a ${rc} != 8 ]; then
    echo "Failed to get response on localhost:${port}"
    exit 1
  fi
}

# pick_unused_port selects an apparently unused port.
pick_unused_port() {
  local avoid=${1:-0}
  local base=6962
  local port
  for (( port = "${base}" ; port <= 61000 ; port++ )); do
    if [[ $port == $avoid ]]; then
      continue
    fi
    if ! lsof -i :$port > /dev/null; then
      echo $port
      break
    fi
  done
}

# kill_pid tries to kill the given pid(s), first softly then more aggressively.
kill_pid() {
  local pids=$@
  set +e
  local count=0
  while kill -INT ${pids} > /dev/null 2>&1; do
    sleep 1
    ((count++))
    local im_still_alive=""
    for pid in ${pids}; do
      if ps -p ${pid} > /dev/null ; then
        # https://www.youtube.com/watch?time_continue=1&v=VuLktUzq23c
        im_still_alive+=" ${pid}"
      fi
    done
    pids="${im_still_alive}"
    if [ -z "${pids}" ]; then
      # all gone!
      break
    fi
    if [ $count -gt 5 ]; then
      echo "Now do kill -KILL ${pids}"
      kill -KILL ${pids}
      break
    fi
    echo "Retry kill -INT ${pids}"
  done
  set -e
}

# log_prep_test prepares a set of running processes for a Trillian log test.
# Parameters:
#   - number of log servers to run
#   - number of log signers to run
# Env:
#   - If TEST_MYSQL_URI is set, uses that for the server --mysql_uri flag.
# Populates:
#  - HTTP_SERVER_1   : first HTTP server
#  - RPC_SERVER_1    : first RPC server
#  - RPC_SERVERS     : RPC target, either comma-separated list of RPC addresses or etcd service
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - LOG_SIGNER_PIDS : bash array of signer pids
#  - ETCD_OPTS       : common option to configure etcd location
# If the ETCD_DIR var points to a valid etcd, also populates:
#  - ETCD_PID        : etcd pid
#  - ETCD_DB_DIR     : location of etcd database
# If WITH_PKCS11 is set, also populates:
#  - SOFTHSM_CONF    : location of the SoftHSM configuration file
#
log_prep_test() {
  # Default to one of each.
  local rpc_server_count=${1:-1}
  local log_signer_count=${2:-1}

  # Wipe the test database
  yes | bash "${TRILLIAN_PATH}/scripts/resetdb.sh"

  local logserver_opts=''
  local logsigner_opts=''
  local has_etcd=0

  if [[ "${TEST_MYSQL_URI}" != "" ]]; then
    logserver_opts+=" --mysql_uri=${TEST_MYSQL_URI}"
    logsigner_opts+=" --mysql_uri=${TEST_MYSQL_URI}"
  fi

  # Start a local etcd instance (if configured).
  if [[ -x "${ETCD_DIR}/etcd" ]]; then
    has_etcd=1
    local etcd_port=2379
    local etcd_server="localhost:${etcd_port}"
    echo "Starting local etcd server on ${etcd_server}"
    ${ETCD_DIR}/etcd &
    ETCD_PID=$!
    ETCD_OPTS="--etcd_servers=${etcd_server}"
    ETCD_DB_DIR=default.etcd
    wait_for_server_startup ${etcd_port}
    logserver_opts="${logserver_opts} --etcd_http_service=trillian-logserver-http --etcd_service=trillian-logserver --quota_system=etcd"
    logsigner_opts="${logsigner_opts} --etcd_http_service=trillian-logsigner-http --quota_system=etcd"
  else
    if  [[ ${log_signer_count} > 1 ]]; then
      echo "*** Warning: running multiple signers with no etcd instance ***"
    fi
    logsigner_opts="${logsigner_opts} --force_master"
  fi

  if [[ "${WITH_PKCS11}" == "true" ]]; then
    export SOFTHSM_CONF=${TMPDIR}/softhsm.conf
    local pkcs11_opts="--pkcs11_module_path ${PKCS11_MODULE:-/usr/lib/softhsm/libsofthsm.so}"
  fi

  # Start a set of Log RPC servers.
  for ((i=0; i < rpc_server_count; i++)); do
    port=$(pick_unused_port)
    RPC_SERVERS="${RPC_SERVERS},localhost:${port}"
    http=$(pick_unused_port ${port})

    echo "Starting Log RPC server on localhost:${port}, HTTP on localhost:${http}"
    go run ${GOFLAGS} github.com/google/trillian/cmd/trillian_log_server \
      ${ETCD_OPTS} ${pkcs11_opts} ${logserver_opts} \
      --rpc_endpoint="localhost:${port}" \
      --http_endpoint="localhost:${http}" \
      ${LOGGING_OPTS} \
      &
    pid=$!
    RPC_SERVER_PIDS+=(${pid})
    wait_for_server_startup ${port}

    # Use the first Log server as the Admin server (any would do)
    if [[ $i -eq 0 ]]; then
      HTTP_SERVER_1="localhost:${http}"
      RPC_SERVER_1="localhost:${port}"
    fi
  done
  RPC_SERVERS="${RPC_SERVERS:1}"

  # Setup etcd quotas, if applicable
  if [[ ${has_etcd} -eq 1 ]]; then
    setup_etcd_quotas "${RPC_SERVER_1}"
  fi

  # Start a set of signers.
  for ((i=0; i < log_signer_count; i++)); do
    port=$(pick_unused_port)
    http=$(pick_unused_port ${port})
    echo "Starting Log signer, HTTP on localhost:${http}"
    go run ${GOFLAGS} github.com/google/trillian/cmd/trillian_log_signer \
      ${ETCD_OPTS} ${pkcs11_opts} ${logsigner_opts} \
      --sequencer_interval="1s" \
      --batch_size=500 \
      --rpc_endpoint="localhost:${port}" \
      --http_endpoint="localhost:${http}" \
      --num_sequencers 2 \
      ${LOGGING_OPTS} \
      &
    pid=$!
    LOG_SIGNER_PIDS+=(${pid})
    wait_for_server_startup ${http}
  done

  if [[ ! -z "${ETCD_OPTS}" ]]; then
    RPC_SERVERS="trillian-logserver"
    echo "Registered log servers @${RPC_SERVERS}/"
    ETCDCTL_API=3 etcdctl get ${RPC_SERVERS}/ --prefix
    echo "Registered HTTP endpoints"
    ETCDCTL_API=3 etcdctl get trillian-logserver-http/ --prefix
    ETCDCTL_API=3 etcdctl get trillian-logsigner-http/ --prefix
  fi
}

# log_stop_tests closes down a set of running processes for a log test.
# Assumes the following variables are set:
#  - LOG_SIGNER_PIDS : bash array of signer pids
#  - RPC_SERVER_PIDS : bash array of RPC server pids
#  - ETCD_PID        : etcd pid
log_stop_test() {
  local pids
  echo "Stopping Log signers (pids ${LOG_SIGNER_PIDS[@]})"
  pids+=" ${LOG_SIGNER_PIDS[@]}"
  echo "Stopping Log RPC servers (pids ${RPC_SERVER_PIDS[@]})"
  pids+=" ${RPC_SERVER_PIDS[@]}"
  if [[ "${ETCD_PID}" != "" ]]; then
    echo "Stopping local etcd server (pid ${ETCD_PID})"
    pids+=" ${ETCD_PID}"
  fi
  kill_pid ${pids}
}

# setup_etcd_quotas creates the etcd quota configurations used by tests.
#
# Parameters:
#   - server : GRPC endpoint for the quota API (eg, logserver grpc port)
#
# Outputs:
#   DeleteConfig and CreateConfig responses.
#
# Returns:
#   0 if success, non-zero otherwise.
setup_etcd_quotas() {
  local server="$1"
  local name='quotas/global/write/config'

  # Remove the config before creating. It's OK if it doesn't exist.
  local delete_output=$(grpcurl -plaintext -d "name: ${name}" ${server} quotapb.Quota.DeleteConfig )
  printf 'quotapb.Quota.DeleteConfig %s: %s\n' "${name}" "${delete_output}"

  local create_output=$(grpcurl -plaintext -d @ ${server} quotapb.Quota.CreateConfig <<EOF
{
  "name": "${name}",
  "config": {
    "state": "ENABLED",
    "max_tokens": 1000,
    "sequencing_based": {
    }
  }
}
EOF
  )
  printf 'quotapb.Quota.CreateConfig %s: %s\n' "${name}" "${create_output}"

  # Success responses have the config name in them
  echo "${create_output}" | grep '"name":' > /dev/null
}

# map_prep_test prepares a set of running processes for a Trillian map test.
# Parameters:
#   - number of map servers to run
# Env:
#   - If TEST_MYSQL_URI is set, uses that for the server --mysql_uri flag.
# Populates:
#  - RPC_SERVER_1    : first RPC server
#  - RPC_SERVERS     : RPC target, either comma-separated list of RPC addresses or etcd service
#  - RPC_SERVER_PIDS : bash array of RPC server pids
map_prep_test() {
  # Default to one map server.
  local rpc_server_count=${1:-1}

  # Wipe the test database
  yes | bash "${TRILLIAN_PATH}/scripts/resetdb.sh"

  local mapserver_opts=''
  if [[ "${TEST_MYSQL_URI}" != "" ]]; then
    mapserver_opts+=" --mysql_uri=${TEST_MYSQL_URI}"
  fi

  # Start a set of Map RPC servers.
  for ((i=0; i < rpc_server_count; i++)); do
    port=$(pick_unused_port)
    RPC_SERVERS="${RPC_SERVERS},localhost:${port}"
    http=$(pick_unused_port ${port})

    echo "Starting Map RPC server on localhost:${port}, HTTP on localhost:${http}"
    go run ${GOFLAGS} github.com/google/trillian/cmd/trillian_map_server \
      ${mapserver_opts} \
      --rpc_endpoint="localhost:${port}" \
      --http_endpoint="localhost:${http}" \
      --single_transaction=true \
      --alsologtostderr \
      &
    pid=$!
    RPC_SERVER_PIDS+=(${pid})
    wait_for_server_startup ${port}

    # Use the first Map server as the Admin server (any would do)
    if [[ $i -eq 0 ]]; then
      RPC_SERVER_1="localhost:${port}"
    fi
  done
  RPC_SERVERS="${RPC_SERVERS:1}"
}

# map_stop_tests closes down a set of running processes for a map test.
# Assumes the following variables are set:
#  - RPC_SERVER_PIDS : bash array of RPC server pids
map_stop_test() {
  echo "Stopping Map RPC servers (pids ${RPC_SERVER_PIDS[@]}"
  kill_pid ${RPC_SERVER_PIDS[@]}
}

# map_provision creates new Trillian maps
# Parameters:
#   - location of admin server instance
#   - number of maps to provision (default: 1)
# Populates:
#  - MAP_IDS: comma-separated list of tree IDs for provisioned maps
map_provision() {
  local admin_server="$1"
  local count=${2:-1}

  echo 'Running createtree'
  for ((i=0; i < count; i++)); do
    local map_id=$(go run ${GOFLAGS} github.com/google/trillian/cmd/createtree \
      --logtostderr \
      --admin_server="${admin_server}" \
      --tree_type=MAP \
      --hash_strategy=TEST_MAP_HASHER \
      --private_key_format=PrivateKey \
      --pem_key_path=${TRILLIAN_PATH}/testdata/map-rpc-server.privkey.pem \
      --pem_key_password=towel)
    echo "Created map ${tree_id}"
    if [[ $i -eq 0 ]]; then
      MAP_IDS="${map_id}"
    else
      MAP_IDS="${MAP_IDS},${map_id}"
    fi
  done
}

# on_exit will clean up anything in ${TO_KILL} and ${TO_DELETE}.
on_exit() {
  local pids=
  for pid in "${TO_KILL[@]}"; do
    echo "Killing ${pid} on exit"
    pids+=" ${pid}"
  done
  kill_pid "${pids}"

  local file=""
  for file in "${TO_DELETE[@]}"; do
    echo "Deleting ${file} on exit"
    rm -rf ${file}
  done
}

trap on_exit EXIT
