#!/bin/bash
set -x
set -e

git clone --depth=1 https://github.com/googleapis/googleapis.git "$GOPATH/src/github.com/googleapis/googleapis"
go install \
    github.com/golang/protobuf/proto \
    github.com/golang/mock/mockgen \
    golang.org/x/tools/cmd/stringer \
    github.com/golang/protobuf/protoc-gen-go \
    github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway \
    github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc
go mod download

# finally, spin up an ephemeral mysql instance for tests
docker-compose -f ./integration/cloudbuild/docker-compose-mysql.yaml pull
docker-compose -f ./integration/cloudbuild/docker-compose-mysql.yaml up -d
# Wait for mysql instance to be ready
while ! mysql --protocol=TCP --host=${MYSQL_HOST} --user=${MYSQL_USER} -p${MYSQL_PASSWORD} -e quit ; do
 sleep 1
done
