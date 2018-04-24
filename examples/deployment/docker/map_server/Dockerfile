FROM golang:1.9

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get ./server/trillian_map_server

ENTRYPOINT ["/go/bin/trillian_map_server"]
