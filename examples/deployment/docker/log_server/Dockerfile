FROM golang:1.11 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ARG GOFLAGS=""
RUN go get ./server/trillian_log_server

FROM gcr.io/distroless/base

COPY --from=build /go/bin/trillian_log_server /
# Include the source code for Trillian and all of its transitive dependencies.
# This ensures that we meet their license requirements to redistribute their
# licenses, copyright notices and, in some cases, source code.
COPY --from=build /go/src /go/src

ENTRYPOINT ["/trillian_log_server"]
