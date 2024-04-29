FROM golang:1.22.2-bookworm@sha256:d0902bacefdde1cf45528c098d14e55d78c107def8a22d148eabd71582d7a99f as build

WORKDIR /trillian

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

# Download dependencies first - this should be cacheable.
COPY go.mod go.sum ./
RUN go mod download

# Now add the local Trillian repo, which typically isn't cacheable.
COPY . .

# Build the server.
RUN go install ./cmd/trillian_log_server
# Run the licensing tool and save licenses, copyright notices, etc.
RUN go run github.com/google/go-licenses save ./cmd/trillian_log_server --save_path /THIRD_PARTY_NOTICES

# Make a minimal image.
FROM gcr.io/distroless/base-debian12@sha256:611d30d7f6d9992c37b1e1a212eefdf1f7c671deb56db3707e24eb01da8c4c2a

COPY --from=build /go/bin/trillian_log_server /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_server"]
