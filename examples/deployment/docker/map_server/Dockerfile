FROM golang:1.10 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ARG GOFLAGS=""
RUN go get ${GOFLAGS} ./server/trillian_map_server

ENTRYPOINT ["/go/bin/trillian_map_server"]
