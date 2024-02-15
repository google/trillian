FROM golang:1.22.0-bookworm@sha256:925fe3fa28ba428cf67a7947ae838f8a1523117b40e3e6b5106c378e3f97fa29 as build

WORKDIR /trillian

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS

# Download dependencies first - this should be cacheable.
COPY go.mod go.sum ./
RUN go mod download

# Now add the local Trillian repo, which typically isn't cacheable.
COPY . .

# Build the signer.
RUN go install ./cmd/trillian_log_signer
# Run the licensing tool and save licenses, copyright notices, etc.
RUN go run github.com/google/go-licenses save ./cmd/trillian_log_signer --save_path /THIRD_PARTY_NOTICES

# Make a minimal image.
FROM gcr.io/distroless/base-debian12@sha256:2102ce121ff1448316b93c5f347118a8e604f5ba7ec9dd7a5c2d8b0eac2941fe

COPY --from=build /go/bin/trillian_log_signer /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_signer"]
