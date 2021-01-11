# This Dockerfile builds a base image for the Trillan CloudBuild integration testing.
FROM ubuntu as trillian_testbase

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on
ENV GOPATH /go

RUN apt-get update && apt-get -y install build-essential curl docker-compose lsof mysql-client socat unzip wget xxd softhsm

RUN curl -sfL https://golang.org/dl/go1.15.7.linux-amd64.tar.gz | tar -xzf - -C /usr/local
ENV PATH /usr/local/go/bin:$PATH

RUN curl -sfL https://install.goreleaser.com/github.com/golangci/golangci-lint.sh | sh -s -- -b $(go env GOPATH)/bin v1.18.0
RUN mkdir protoc && \
    (cd protoc && \
    PROTOC_VERSION="3.11.4" && \
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    wget "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}" && \
    unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc && \
    unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*' \
    )
ENV PATH /usr/local/bin:$PATH
RUN BAZEL_VERSION="1.1.0" && \
    wget --quiet -O install.sh "https://github.com/bazelbuild/bazel/releases/download/${BAZEL_VERSION}/bazel-${BAZEL_VERSION}-installer-linux-x86_64.sh" && \
    chmod +x install.sh && \
    ./install.sh

ENV PATH $GOPATH/bin:/testbase/protoc/bin:$PATH