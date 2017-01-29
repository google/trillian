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
misspell . -locale US
regenerate
trap - EXIT
go build ./...
go test -cover ${GOFLAGS} ./...
find . -type d -exec gocyclo -over 25 {} \;
