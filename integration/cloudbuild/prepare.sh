#!/bin/bash

set -ex

git clone --depth=1 https://github.com/googleapis/googleapis.git "$GOPATH/src/github.com/googleapis/googleapis"

go install \
    github.com/golang/protobuf/proto \
    github.com/golang/mock/mockgen \
    golang.org/x/tools/cmd/stringer \
    github.com/golang/protobuf/protoc-gen-go \
    github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc

# Cache MySQL image for later steps.
docker-compose -f ./integration/cloudbuild/docker-compose-mysql.yaml pull
