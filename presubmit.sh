#!/bin/bash
set -ex

find . -name "*.pb.go" -delete
find . -name "mock_*" -delete
golint --set_exit_status ./...
go vet ./...
go generate -run="protoc" ./...
go generate -run="mockgen" ./...
go build ./...
go test -cover ${GOFLAGS} ./...
find . -type d -exec gocyclo -over 25 {} \;
find . -print0 | xargs -0 -P 10 misspell -locale US -locale UK

