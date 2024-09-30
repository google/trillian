FROM golang:1.23.1-bookworm@sha256:dba79eb312528369dea87532a65dbe9d4efb26439a0feacc9e7ac9b0f1c7f607 as build

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
RUN go run github.com/google/go-licenses/v2 save ./cmd/trillian_log_signer --save_path /THIRD_PARTY_NOTICES

# Make a minimal image.
FROM gcr.io/distroless/base-debian12@sha256:6ae5fe659f28c6afe9cc2903aebc78a5c6ad3aaa3d9d0369760ac6aaea2529c8

COPY --from=build /go/bin/trillian_log_signer /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_signer"]
