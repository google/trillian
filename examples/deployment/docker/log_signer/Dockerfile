FROM golang:1.11 as build

WORKDIR /trillian

ARG GOFLAGS=""
ENV GOFLAGS=$GOFLAGS
ENV GO111MODULE=on

# Download dependencies first - this should be cacheable.
COPY go.mod go.sum ./
RUN go mod download

# Now add the local Trillian repo, which typically isn't cacheable.
COPY . .

# Build the signer and licensing tool.
RUN go get ./server/trillian_log_signer ./scripts/licenses
# Run the licensing tool and save licenses, copyright notices, etc.
RUN licenses save ./server/trillian_log_signer --save_path /THIRD_PARTY_NOTICES

# Make a minimal image.
FROM gcr.io/distroless/base

COPY --from=build /go/bin/trillian_log_signer /
COPY --from=build /THIRD_PARTY_NOTICES /THIRD_PARTY_NOTICES

ENTRYPOINT ["/trillian_log_signer"]
