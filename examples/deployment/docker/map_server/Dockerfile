FROM golang:1.11 as build

ADD . /go/src/github.com/google/trillian
WORKDIR /go/src/github.com/google/trillian

ARG GOFLAGS=""
RUN go get ./server/trillian_map_server ./scripts/licenses
RUN licenses save ./server/trillian_map_server --save_path /THIRD_PARTY_NOTICES

FROM gcr.io/distroless/base

COPY --from=build /go/bin/trillian_map_server /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_map_server"]
