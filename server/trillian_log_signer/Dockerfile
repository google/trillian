FROM golang:1.8

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN apt-get update && \
    apt-get install -y libtool libltdl-dev

RUN go get ./server/trillian_log_signer

ENTRYPOINT ["/go/bin/trillian_log_signer"]
