#!/bin/bash

PROTODIRS=" $(find . -name \*.proto -printf '%h\n' | sort | uniq | xargs echo)"
INCDIRS=${PROTODIRS// / -I }
for DIR in ${PROTODIRS}; do
  CMD="protoc --go_out=plugins=grpc:. ${INCDIRS} ${DIR}/*.proto"
  echo $CMD
  $CMD
done
