# This Dockerfile builds a base image for Trillan integration tests.
FROM ubuntu

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on
ENV GOPATH /go

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  docker-compose \
  lsof \
  mysql-client \
  socat \
  softhsm \
  unzip \
  wget \
  xxd

RUN curl -sfL https://golang.org/dl/go1.15.7.linux-amd64.tar.gz | tar -xzf - -C /usr/local
ENV PATH /usr/local/go/bin:$PATH

# https://golangci-lint.run/usage/install/
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.36.0

RUN mkdir protoc && \
    (cd protoc && \
    PROTOC_VERSION="3.12.4" && \
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    wget "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}" && \
    unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc && \
    unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*' \
    )
ENV PATH /usr/local/bin:$PATH

ENV PATH $GOPATH/bin:/testbase/protoc/bin:$PATH
