FROM golang:1.11 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ARG GOFLAGS=""
RUN go get ./server/trillian_log_signer

FROM gcr.io/distroless/base

COPY --from=build /go/bin/trillian_log_signer /

ENTRYPOINT ["/trillian_log_signer"]
