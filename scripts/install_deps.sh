#!/bin/bash
#
# Installs the dependencies required to build this repo.
set -eu

main() {
  go install github.com/golang/mock/mockgen
  go install google.golang.org/protobuf/proto
  go install google.golang.org/protobuf/cmd/protoc-gen-go
  go install google.golang.org/grpc/cmd/protoc-gen-go-grpc
  go install github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
  go install golang.org/x/tools/cmd/stringer
}

main
