# This Dockerfile builds a base image for Trillan integration tests.
FROM buildbase

WORKDIR /testbase

RUN apt-get update && apt-get install -y \
  docker-compose \
  lsof \
  mysql-client \
  socat \
  softhsm \
  xxd

ENV PATH $GOPATH/bin:/testbase/protoc/bin:$PATH
