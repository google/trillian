export INTEGRATION_DIR="$( cd "$( dirname "$0" )" && pwd )"
export TRILLIAN_ROOT="${INTEGRATION_DIR}"/..
export SCRIPTS_DIR="${TRILLIAN_ROOT}"/scripts
export TESTDATA="${TRILLIAN_ROOT}"/testdata
export TESTDBOPTS="-u test --password=zaphod -D test"
export STARTUP_WAIT_SECONDS=10

function runTest() {
  echo "=== RUN   ${2}"
  "${INTEGRATION_DIR}/$1"
  rc=$?
  if [ $rc -ne 0 ]; then
    echo "--- FAIL: ${2}"
    return $rc
  fi
  echo "--- PASS: ${2}"
  return 0
}

# Wait for a server to become ready
function waitForServerStartup() {
  # The server will 404 the request as there's no handler for it. This error doesn't matter
  # as the test will fail if the server is really not up.
  PORT=$1
  wget -q --spider --retry-connrefused --waitretry=1 -t ${STARTUP_WAIT_SECONDS} localhost:${PORT}
  # Wait a bit more to give it a chance to become actually available e.g. if Travis is slow
  sleep 2
}


# Clean up anything in ${TO_KILL} and ${TO_DELETE}.
function onExit() {
  if [[ "${TO_KILL}" ]]; then
    echo "Killing ${TO_KILL} on exit"
    kill -INT ${TO_KILL}
  fi
  if [[ "${TO_DELETE}" ]]; then
    echo "Deleting ${TO_DELETE} on exit"
    rm ${TO_DELETE}
  fi
}

trap onExit EXIT
