#!/bin/bash
set -ex

regenerate() {
    go generate -run="protoc" ./...
    go generate -run="mockgen" ./...
}
trap regenerate EXIT

find . -name "*.pb.go" -delete
find . -name "mock_*" -delete
golint --set_exit_status ./...
go vet ./...
misspell -error -locale US .
nolicense=$(find . -name '*.go' -or -name '*.proto' | grep -v ./protoc | xargs grep -L "Apache License")
if [[ "${nolicense}" ]]; then
    echo "Missing license header in: ${nolicense}"
    exit 1
fi
regenerate
trap - EXIT
go build ./...
go test -cover ${GOFLAGS} ./...
find . -type d -exec gocyclo -over 25 {} \;
