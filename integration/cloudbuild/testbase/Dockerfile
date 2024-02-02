# This Dockerfile builds a base image for Trillan integration tests.
FROM golang:1.21.6-bookworm@sha256:3efef61ff1d99c8a90845100e2a7e934b4a5d11b639075dc605ff53c141044fc

WORKDIR /testbase

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GOPATH /go

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  docker-compose \
  lsof \
  mariadb-client \
  netcat-openbsd \
  socat \
  softhsm \
  unzip \
  wget \
  xxd

# Install golangci-lint. See docs at: https://golangci-lint.run/usage/install/.
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.55.1

# Install CockroachDB, see https://www.cockroachlabs.com/docs/v22.2/install-cockroachdb-linux
RUN curl https://binaries.cockroachdb.com/cockroach-v22.2.7.linux-amd64.tgz | tar -xz && cp -i cockroach-v22.2.7.linux-amd64/cockroach /usr/local/bin/

RUN mkdir protoc && \
    (cd protoc && \
    PROTOC_VERSION="3.20.1" && \
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    wget "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}" && \
    unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc && \
    unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*' \
    )
ENV PATH /usr/local/bin:$PATH

ENV PATH $GOPATH/bin:/testbase/protoc/bin:$PATH
