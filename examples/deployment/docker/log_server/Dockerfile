FROM golang:1.21.5-bookworm@sha256:a0e920ad62a507721a5dd6fce48deac89a1f7a7dc0c0723cb539a564b0eb2a72 as build

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
FROM gcr.io/distroless/base-debian12@sha256:8a0bb635ccf4cfa88b2bae707c82e633d0867b77827054ed146ad5e13e35ce79

COPY --from=build /go/bin/trillian_log_server /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_server"]
