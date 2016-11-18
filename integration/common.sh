export INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export TRILLIAN_ROOT=${INTEGRATION_DIR}/..
export SCRIPTS_DIR=${TRILLIAN_ROOT}/scripts
export TESTDATA=${TRILLIAN_ROOT}/testdata
export TESTDBOPTS="-u test --password=zaphod -D test"
export STARTUP_WAIT_SECONDS=10

function runTest() {
  echo "=== RUN   ${2}"
  ${INTEGRATION_DIR}/$1
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
  PORT=$1
  wget -q --spider --retry-connrefused --waitretry=1 -t ${STARTUP_WAIT_SECONDS} localhost:${PORT}
  # Wait a bit more to give it a chance to become actually available e.g. if Travis is slow
  sleep 2
}
