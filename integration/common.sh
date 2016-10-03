export INTEGRATION_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
export TRILLIAN_ROOT=${INTEGRATION_DIR}/..
export TESTDATA=${TRILLIAN_ROOT}/testdata

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


