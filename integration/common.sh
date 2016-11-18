export INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export TRILLIAN_ROOT=${INTEGRATION_DIR}/..
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

# Wipe all Log storage rows for the given tree ID.
function wipeLog() {
  TREE_ID=$1
  mysql ${TESTDBOPTS} -e "DELETE FROM Unsequenced WHERE TreeId = ${TREE_ID}"
  mysql ${TESTDBOPTS} -e "DELETE FROM TreeHead WHERE TreeId = ${TREE_ID}"
  mysql ${TESTDBOPTS} -e "DELETE FROM SequencedLeafData WHERE TreeId = ${TREE_ID}"
  mysql ${TESTDBOPTS} -e "DELETE FROM LeafData WHERE TreeId = ${TREE_ID}"
  mysql ${TESTDBOPTS} -e "DELETE FROM Trees WHERE TreeId = ${TREE_ID}"
}

# Create a new Log storage row for the given tree ID.
function createLog() {
  TREE_ID=$1
  mysql ${TESTDBOPTS} -e "INSERT INTO Trees VALUES (${TREE_ID}, 1, 'LOG', 'SHA256', 'SHA256', false)"
}

# Wipe all Map storage rows for the given tree ID.
function wipeMap() {
  TREE_ID=$1
  mysql ${TESTDBOPTS} -e "DELETE FROM Trees WHERE TreeId = ${TREE_ID}"
}

# Create a new Map storage row for the given tree ID.
function createMap() {
  TREE_ID=$1
  mysql ${TESTDBOPTS} -e "INSERT INTO Trees VALUES (${TREE_ID}, 1, 'MAP', 'SHA256', 'SHA256', false)"
}

# Wait for a server to become ready
function waitForServerStartup() {
  PORT=$1
  wget -q --spider --retry-connrefused --waitretry=1 -t ${STARTUP_WAIT_SECONDS} localhost:${PORT}
  # Wait a bit more to give it a chance to become actually available e.g. if Travis is slow
  sleep 2
}