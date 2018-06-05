FROM golang:1.10 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

RUN go get ./server/trillian_log_server

FROM gcr.io/distroless/base

LABEL maintainer="Trillian Team <email@address.com>"

COPY --from=build /go/bin/trillian_log_server /

ENTRYPOINT ["/trillian_log_server"]
