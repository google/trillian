# This Dockerfile builds a base image for Trillan integration tests.
FROM ubuntu

WORKDIR /workspace

ARG GOFLAGS=""

ENV GO_VERSION="1.15.7"
ENV GOLANGCI_VERSION="v1.36.0"
ENV PROTOC_VERSION="3.12.4"

ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on
ENV GOPATH /go

RUN apt-get update && apt-get install -y \
  build-essential \
  curl \
  git \
  unzip \
  wget

RUN curl -sfL https://golang.org/dl/go${GO_VERSION}.linux-amd64.tar.gz | tar -xzf - -C /usr/local
ENV PATH /usr/local/go/bin:$PATH

# Install golangci-lint. See docs at: https://golangci-lint.run/usage/install/.
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin ${GOLANGCI_VERSION}

RUN mkdir /protoc && \
    (cd /protoc && \
    PROTOC_ZIP="protoc-${PROTOC_VERSION}-linux-x86_64.zip" && \
    wget "https://github.com/google/protobuf/releases/download/v${PROTOC_VERSION}/${PROTOC_ZIP}" && \
    unzip -o ${PROTOC_ZIP} -d /usr/local bin/protoc && \
    unzip -o ${PROTOC_ZIP} -d /usr/local 'include/*' \
    )

RUN git clone --depth=1 https://github.com/googleapis/googleapis.git "$GOPATH/src/github.com/googleapis/googleapis"

COPY go.* /workspace/
RUN go mod download

# Install the tooling used for auto-generating code. Specifically, these are the
# tools mentioned in //go:generate comments throughout this repository, and used
# by the "go generate" command. In CI this is used for ensuring that developers
# commit the generated files in an up-to-date state.
RUN go install \
    github.com/golang/mock/mockgen \
    github.com/golang/protobuf/proto \
    github.com/golang/protobuf/protoc-gen-go \
    github.com/pseudomuto/protoc-gen-doc/cmd/protoc-gen-doc \
    golang.org/x/tools/cmd/goimports \
    golang.org/x/tools/cmd/stringer

ENV PATH $GOPATH/bin:/protoc/bin:/usr/local/bin:$PATH