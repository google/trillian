#!/bin/bash
pushd ../../../
protoc --go_out=plugins=grpc:. github.com/google/trillian/*.proto
protoc --go_out=plugins=grpc:. github.com/google/trillian/storage/*.proto
popd

