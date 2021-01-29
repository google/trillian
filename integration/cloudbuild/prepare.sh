#!/bin/bash

set -ex

# TODO(pavelkalinnikov): This script can be made a definitive "how to" for
# setting up dev environment. Eliminate duplicating these steps in many places.

# Install Google API definitions. Some APIs are used by the protoc tool when
# [re-]generating Trillian API packages from protobufs.
#
# TODO(pavelkalinnikov): It doesn't have to be within $GOPATH. There is no Go
# code/module in this repository, and we use it only for API proto definitions.
git clone --depth=1 https://github.com/googleapis/googleapis.git "$GOPATH/src/github.com/googleapis/googleapis"

# Install the tooling used for auto-generating code. Specifically, these are the
# tools mentioned in //go:generate comments throughout this repository, and used
# by the "go generate" command. In CI this is used for ensuring that developers
# commit the generated files in an up-to-date state.
go install \
    github.com/golang/mock/mockgen \
    github.com/golang/protobuf/proto \
    github.com/golang/protobuf/protoc-gen-go \
    github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc \
    golang.org/x/tools/cmd/stringer

# Cache MySQL image for later steps.
# TODO(pavelkalinnikov): Specific to MySQL tests. Move to a better place.
docker-compose -f ./integration/cloudbuild/docker-compose-mysql.yaml pull
