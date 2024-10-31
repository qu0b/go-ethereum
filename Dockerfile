# Support setting various labels on the final image
ARG COMMIT=""
ARG VERSION=""
ARG BUILDNUM=""

# Build Geth in a stock Go builder container
FROM golang:1.23.2-bookworm AS builder

RUN apt-get update
RUN apt-get install -y linux-headers-6.1.0-26-amd64 gcc git bash

# Get dependencies - will also be cached if we won't change go.mod/go.sum
COPY go.mod /go-ethereum/
COPY go.sum /go-ethereum/
RUN cd /go-ethereum && go mod download

ADD . /go-ethereum
RUN go install github.com/antithesishq/antithesis-sdk-go/tools/antithesis-go-instrumentor@latest
RUN cd /go-ethereum && go mod tidy
# breaks if I uncomment this
RUN antithesis-go-instrumentor -assert_only -catalog_dir=/go-ethereum/cmd/geth /go-ethereum
RUN cd /go-ethereum && CGO_ENABLED=1 go run build/ci.go install ./cmd/geth

# Pull Geth into a second stage deploy alpine container
FROM ubuntu:latest

RUN apt-get update
RUN apt-get install -y ca-certificates
# RUN apk add --no-cache ca-certificates

COPY --from=builder /go-ethereum/build/bin/geth /usr/local/bin/

EXPOSE 8545 8546 30303 30303/udp
ENTRYPOINT ["geth"]

# Add some metadata labels to help programmatic image consumption
ARG COMMIT=""
ARG VERSION=""
ARG BUILDNUM=""

LABEL commit="$COMMIT" version="$VERSION" buildnum="$BUILDNUM"
