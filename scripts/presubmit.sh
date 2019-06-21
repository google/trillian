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

  go_srcs="$(find . -name '*.go' | \
    grep -v mock_ | \
    grep -v .pb.go | \
    grep -v .pb.gw.go | \
    grep -v _string.go | \
    tr '\n' ' ')"

  if [[ "$fix" -eq 1 ]]; then
    check_pkg goimports golang.org/x/tools/cmd/goimports || exit 1

    echo 'running gofmt'
    gofmt -s -w ${go_srcs}
    echo 'running goimports'
    goimports -w ${go_srcs}
  fi

  if [[ "${run_build}" -eq 1 ]]; then
    echo 'running go build'
    go build ./...

    echo 'running go test'
    # Install test deps so that individual test runs below can reuse them.
    echo 'installing test deps'
    go test -i ./...

    if [[ ${coverage} -eq 1 ]]; then
        local coverflags="-covermode=atomic -coverprofile=coverage.txt"

        go test \
            -short \
            -timeout=${GO_TEST_TIMEOUT:-5m} \
            ${coverflags} \
	    ./... -alsologtostderr
    else
      go test \
        -short \
        -timeout=${GO_TEST_TIMEOUT:-5m} \
        ./... -alsologtostderr
    fi
  fi

  if [[ "${run_lint}" -eq 1 ]]; then
    check_cmd golangci-lint \
      'have you installed github.com/golangci/golangci-lint?' || exit 1
    check_cmd prototool \
      'have you installed github.com/uber/prototool/cmd/prototool?' || exit 1

    echo 'running golangci-lint'
    golangci-lint run --deadline=8m
    echo 'running prototool lint'
    prototool lint
    echo 'checking license headers'
    ./scripts/check_license.sh ${go_srcs}
  fi

  if [[ "${run_generate}" -eq 1 ]]; then
    check_cmd protoc 'have you installed protoc?'
    check_pkg mockgen github.com/golang/mock/mockgen || exit 1
    check_pkg stringer golang.org/x/tools/cmd/stringer || exit 1

    echo 'running go generate'
    go generate -run="protoc" ./...
    go generate -run="mockgen" ./...
    go generate -run="stringer" ./...
  fi
}

main "$@"
