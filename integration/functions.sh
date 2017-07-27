# Functions for setting up Trillian integration tests

if [[ -z "${TMPDIR}" ]]; then
  TMPDIR=/tmp
fi
readonly TMPDIR
declare -a RPC_SERVER_PIDS
declare -a LOG_SIGNER_PIDS
declare -a TO_KILL
declare -a TO_DELETE
ADMIN_SERVER=''
RPC_SERVER_1=''
RPC_SERVERS=''
ETCD_OPTS=''
ETCD_PID=''
ETCD_DB_DIR=''
readonly TRILLIAN_PATH=$(go list -f '{{.Dir}}' github.com/google/trillian)

# run_test runs the given test with additional output messages.
run_test() {
  local name=$1
  local script=$2
  echo "=== RUN   ${name}"
  "${script}" "$3" "$4" "$5" "$6" "$7" "$8"
  rc=$?
  if [ $rc -ne 0 ]; then
    echo "--- FAIL: ${name}"
    return $rc
  fi
  echo "--- PASS: ${name}"
  return 0
}

# wait_for_server_startup pauses until there is a response on the given port.
wait_for_server_startup() {
  # The server will 404 the request as there's no handler for it. This error doesn't matter
  # as the test will fail if the server is really not up.
  local port=$1
  set +e
  wget -q --spider --retry-connrefused --waitretry=1 -t 10 localhost:${port}
  set -e
  # Wait a bit more to give it a chance to become actually available e.g. if Travis is slow
  sleep 2
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

# kill_pid tries to kill the given pid, first softly then more aggressively.
kill_pid() {
  local pid=$1
  set +e
  local count=0
  while kill -INT ${pid} > /dev/null 2>&1; do
    sleep 1
    ((count++))
    if ! ps -p ${pid} > /dev/null ; then
      break
    fi
    if [ $count -gt 5 ]; then
      echo "Now do kill -KILL ${pid}"
      kill -KILL ${pid}
      break
    fi
    echo "Retry kill -INT ${pid}"
  done
  set -e
}

# log_prep_test prepares a set of running processes for a Trillian log test.
# Parameters:
#   - number of log servers to run
#   - number of log signers to run
# Populates:
#  - ADMIN_SERVER    : address for an admin server
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
log_prep_test() {
  # Default to one of each.
  local rpc_server_count=${1:-1}
  local log_signer_count=${2:-1}

  echo "Building Trillian log code"
  go build ${GOFLAGS} github.com/google/trillian/server/trillian_log_server/
  go build ${GOFLAGS} github.com/google/trillian/server/trillian_log_signer/

  # Wipe the test database
  yes | "${TRILLIAN_PATH}/scripts/resetdb.sh"

  # Start a local etcd instance (if configured).
  if [[ -x "${ETCD_DIR}/etcd" ]]; then
    local etcd_port=2379
    local etcd_server="localhost:${etcd_port}"
    echo "Starting local etcd server on ${etcd_server}"
    ${ETCD_DIR}/etcd &
    ETCD_PID=$!
    ETCD_OPTS="--etcd_servers=${etcd_server}"
    ETCD_DB_DIR=default.etcd
    wait_for_server_startup ${etcd_port}
    local logserver_opts="--etcd_http_service=trillian-logserver-http --etcd_service=trillian-logserver"
    local logsigner_opts="--etcd_http_service=trillian-logsigner-http"
  else
    if  [[ ${log_signer_count} > 1 ]]; then
      echo "*** Warning: running multiple signers with no etcd instance ***"
    fi
    local logserver_opts=
    local logsigner_opts="--force_master"
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
    ./trillian_log_server ${ETCD_OPTS} ${pkcs11_opts} ${logserver_opts} --rpc_endpoint="localhost:${port}" --http_endpoint="localhost:${http}" &
    pid=$!
    RPC_SERVER_PIDS+=(${pid})
    wait_for_server_startup ${port}

    # Use the first Log server as the Admin server (any would do)
    if [[ $i -eq 0 ]]; then
      RPC_SERVER_1="localhost:${port}"
    fi
  done
  RPC_SERVERS="${RPC_SERVERS:1}"

  # Start a set of signers.
  for ((i=0; i < log_signer_count; i++)); do
    http=$(pick_unused_port)
    echo "Starting Log signer, HTTP on localhost:${http}"
    ./trillian_log_signer ${ETCD_OPTS} ${pkcs11_opts} ${logsigner_opts} --sequencer_interval="1s" --batch_size=500 --http_endpoint="localhost:${http}" --num_sequencers 2 &
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
  for pid in "${LOG_SIGNER_PIDS[@]}"; do
    echo "Stopping Log signer (pid ${pid})"
    kill_pid ${pid}
  done
  for pid in "${RPC_SERVER_PIDS[@]}"; do
    echo "Stopping Log RPC server (pid ${pid})"
    kill_pid ${pid}
  done
  if [[ "${ETCD_PID}" != "" ]]; then
    echo "Stopping local etcd server (pid ${ETCD_PID})"
    kill_pid ${ETCD_PID}
  fi
}

# on_exit will clean up anything in ${TO_KILL} and ${TO_DELETE}.
on_exit() {
  local pid=0
  for pid in "${TO_KILL[@]}"; do
    echo "Killing ${pid} on exit"
    kill_pid "${pid}"
  done
  local file=""
  for file in "${TO_DELETE[@]}"; do
    echo "Deleting ${file} on exit"
    rm -rf ${file}
  done
}

trap on_exit EXIT
