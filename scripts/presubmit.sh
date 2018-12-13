#!/bin/bash
#
# Presubmit checks for Trillian.
#
# Checks for lint errors, spelling, licensing, correct builds / tests and so on.
# Flags may be specified to allow suppressing of checks or automatic fixes, try
# `scripts/presubmit.sh --help` for details.
#
# Globals:
#   GO_TEST_TIMEOUT: timeout for 'go test'. Optional (defaults to 5m).
set -eu


# Retries running a command N times, with exponential backoff between failures.
#
# Usage:
#   retry N command ...args
retry() {
  local retries=$1
  shift

  local count=0
  until "$@"; do
    local exit=$?
    local wait=$((2 ** $count))
    local count=$(($count + 1))
    if [ $count -lt $retries ]; then
      echo "Attempt $count/$retries: $1 exited $exit, retrying in $wait seconds..."
      sleep $wait
    else
      echo "Attempt $count/$retries: $1 exited $exit, no more retries left."
      return $exit
    fi
  done
  return 0
}

check_pkg() {
  local cmd="$1"
  local pkg="$2"
  check_cmd "$cmd" "try running 'go get -u $pkg'"
}

check_cmd() {
  local cmd="$1"
  local msg="$2"
  if ! type -p "${cmd}" > /dev/null; then
    echo "${cmd} not found, ${msg}"
    return 1
  fi
}

usage() {
  echo "$0 [--coverage] [--fix] [--no-build] [--no-linters] [--no-generate]"
}

main() {
  local coverage=0
  local fix=0
  local run_build=1
  local run_lint=1
  local run_generate=1
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --coverage)
        coverage=1
        ;;
      --fix)
        fix=1
        ;;
      --help)
        usage
        exit 0
        ;;
      --no-build)
        run_build=0
        ;;
      --no-linters)
        run_lint=0
        ;;
      --no-generate)
        run_generate=0
        ;;
      *)
        usage
        exit 1
        ;;
    esac
    shift 1
  done

  cd "$(dirname "$0")"  # at scripts/
  cd ..  # at top level

  if [[ "$fix" -eq 1 ]]; then
    check_pkg goimports golang.org/x/tools/cmd/goimports || exit 1

    local go_srcs="$(find . -name '*.go' | \
      grep -v vendor/ | \
      grep -v mock_ | \
      grep -v .pb.go | \
      grep -v .pb.gw.go | \
      grep -v _string.go | \
      tr '\n' ' ')"

    echo 'running gofmt'
    gofmt -s -w ${go_srcs}
    echo 'running goimports'
    goimports -w ${go_srcs}
  fi

  if [[ "${run_build}" -eq 1 ]]; then
    local goflags=''
    if [[ "${GOFLAGS:+x}" ]]; then
      goflags="${GOFLAGS}"
    fi

    echo 'running go build'
    go build ${goflags} ./...

    echo 'running go test'
    # Install test deps so that individual test runs below can reuse them.
    echo 'installing test deps'
    go test ${goflags} -i ./...

    if [[ ${coverage} -eq 1 ]]; then
        local coverflags="-covermode=atomic -coverprofile=coverage.txt"

        go test \
            -short \
            -timeout=${GO_TEST_TIMEOUT:-5m} \
            ${coverflags} \
            ${goflags} \
	    ./... -alsologtostderr
    else
      go test \
        -short \
        -timeout=${GO_TEST_TIMEOUT:-5m} \
        ${goflags} \
        ./... -alsologtostderr
    fi
  fi

  if [[ "${run_lint}" -eq 1 ]]; then
    check_cmd gometalinter \
      'have you installed github.com/alecthomas/gometalinter?' || exit 1

    echo 'running gometalinter'
    retry 5 gometalinter --config=gometalinter.json --deadline=2m ./...
  fi

  if [[ "${run_generate}" -eq 1 ]]; then
    check_cmd protoc 'have you installed protoc?'
    check_pkg mockgen github.com/golang/mock/mockgen || exit 1
    check_pkg stringer golang.org/x/tools/cmd/stringer || exit 1

    echo 'running go generate'
    go generate -run="protoc" ./...
    # go generate -run="mockgen" ./... # TODO(gbelvin) reenable.
    go generate -run="stringer" ./...
  fi
}

main "$@"
