#!/bin/bash
#
# Presubmit checks for Trillian.
# Checks for lint errors, spelling, licensing, correct builds / tests and so on.
# Flags may be specified to allow suppressing of checks or automatic fixes, try
# `scripts/presubmit.sh --help` for details.
set -eu

check_deps() {
  local failed=0
  check_cmd golint github.com/golang/lint/golint || failed=10
  check_cmd misspell github.com/client9/misspell/cmd/misspell || failed=11
  check_cmd gocyclo github.com/fzipp/gocyclo || failed=12
  check_cmd stringer golang.org/x/tools/cmd/stringer || failed=13
  return $failed
}

check_cmd() {
  local cmd="$1"
  local repo="$2"
  if ! type -p "${cmd}" > /dev/null; then
    echo "${cmd} not found, try to 'go get -u ${repo}'"
    return 1
  fi
}

usage() {
  echo "$0 [--fix] [--no-build] [--no-linters] [--no-generate]"
}

main() {
  check_deps

  local fix=0
  local run_build=1
  local run_linters=1
  local run_generate=1
  while [[ $# -gt 0 ]]; do
    case "$1" in
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
        run_linters=0
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

  local go_dirs="$(go list ./... | \
    grep -v /vendor/)"
  local go_srcs="$(find . -name '*.go' | \
    grep -v mock_ | \
    grep -v .pb.go | \
    grep -v .pb.gw.go | \
    grep -v _string.go | \
    grep -v vendor/ | \
    tr '\n' ' ')"
  local proto_srcs="$(find . -name '*.proto' | \
    grep -v vendor/ | \
    tr '\n' ' ')"

  if [[ "$fix" -eq 1 ]]; then
    check_cmd goimports golang.org/x/tools/cmd/goimports

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
    go build ${goflags} ${go_dirs}

    echo 'running go test'
    go test -cover -timeout=5m -short ${goflags} ${go_dirs}
  fi

  if [[ "${run_linters}" -eq 1 ]]; then
    echo 'running golint'
    printf '%s\n' ${go_srcs} | xargs -I'{}' golint --set_exit_status '{}'

    echo 'running go vet'
    printf '%s\n' ${go_srcs} | xargs -I'{}' go vet '{}'

    echo 'running gocyclo'
    printf '%s\n' ${go_srcs} | xargs -I'{}' bash -c 'gocyclo -over 25 {}'

    echo 'running misspell'
    printf '%s\n' ${go_srcs} | xargs -I'{}' misspell -error -i cancelled,CANCELLED -locale US '{}'

    echo 'checking license header'
    local nolicense="$(grep -L 'Apache License' ${go_srcs} ${proto_srcs})"
    if [[ "${nolicense}" ]]; then
      echo "Missing license header in: ${nolicense}"
      exit 2
    fi
  fi

  if [[ "${run_generate}" -eq 1 ]]; then
    echo 'running go generate'
    go generate -run="protoc" ${go_dirs}
    go generate -run="mockgen" ${go_dirs}
    go generate -run="stringer" ${go_dirs}
  fi

}

main "$@"
