FROM golang:1.11 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ARG GO111MODULE=auto
ENV GO111MODULE=$GO111MODULE
RUN go get ./server/trillian_log_server ./scripts/licenses
RUN licenses save ./server/trillian_log_server --save_path /THIRD_PARTY_NOTICES

FROM gcr.io/distroless/base

COPY --from=build /go/bin/trillian_log_server /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_server"]
