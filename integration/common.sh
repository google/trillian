export INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
export TRILLIAN_ROOT="${INTEGRATION_DIR}"/..
export SCRIPTS_DIR="${TRILLIAN_ROOT}"/scripts
export TESTDATA="${TRILLIAN_ROOT}"/testdata
export TESTDBOPTS="-u test --password=zaphod -D test"
export STARTUP_WAIT_SECONDS=10

function runTest() {
  local name=$1
  local script=$2
  echo "=== RUN   ${name}"
  "${INTEGRATION_DIR}"/"${script}" "$3" "$4" "$5" "$6" "$7" "$8"
  rc=$?
  if [ $rc -ne 0 ]; then
    echo "--- FAIL: ${name}"
    return $rc
  fi
  echo "--- PASS: ${name}"
  return 0
}

# Wait for a server to become ready
function waitForServerStartup() {
  # The server will 404 the request as there's no handler for it. This error doesn't matter
  # as the test will fail if the server is really not up.
  local PORT=$1
  wget -q --spider --retry-connrefused --waitretry=1 -t ${STARTUP_WAIT_SECONDS} localhost:${PORT}
  # Wait a bit more to give it a chance to become actually available e.g. if Travis is slow
  sleep 2
}

function killPid() {
  local pid=$1
  set +e
  while kill -INT ${pid} > /dev/null
  do
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

# Clean up anything in ${TO_KILL} and ${TO_DELETE}.
declare -a TO_KILL
declare -a TO_DELETE
function onExit() {
  for pid in "${TO_KILL[@]}"
  do
    echo "Killing ${pid} on exit"
    killPid "${pid}"
  done
  for file in "${TO_DELETE[@]}"
  do
    echo "Deleting ${file} on exit"
    rm ${file}
  done
}

trap onExit EXIT
