# syntax=docker/dockerfile:1
FROM golang:1.26.2-alpine

RUN apk --no-cache add git ca-certificates gcc musl-dev libc-dev binutils-gold curl openssh make bash

RUN curl -sSfL https://golangci-lint.run/install.sh \
        | sh -s -- -b "$(go env GOPATH)/bin" v2.12.1 \
    && golangci-lint --version

RUN go install github.com/vektra/mockery/v2@v2.53.2

WORKDIR /app
