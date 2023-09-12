FROM golang:1.20.8-bookworm@sha256:b099af7250d673d249597b79049e3e53487c435acf285e07a36d4b49f9f4750a as build

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
FROM gcr.io/distroless/base-debian12@sha256:d64f5483d2fd0cec2260941c443cb2947102e46e1a9fe36a321d0a788c1a49e0

COPY --from=build /go/bin/trillian_log_signer /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_signer"]
