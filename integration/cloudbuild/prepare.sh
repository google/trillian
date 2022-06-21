#!/bin/bash

set -ex

# TODO(pavelkalinnikov): This script can be made a definitive "how to" for
# setting up dev environment. Eliminate duplicating these steps in many places.

# Install the tooling used for auto-generating code. Specifically, these are the
# tools mentioned in //go:generate comments throughout this repository, and used
# by the "go generate" command. In CI this is used for ensuring that developers
# commit the generated files in an up-to-date state.
go install \
    github.com/golang/mock/mockgen \
    google.golang.org/protobuf/proto \
    google.golang.org/protobuf/cmd/protoc-gen-go \
    google.golang.org/grpc/cmd/protoc-gen-go-grpc \
    github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc \
    golang.org/x/tools/cmd/goimports \
    golang.org/x/tools/cmd/stringer
