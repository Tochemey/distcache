VERSION 0.8

FROM golang:1.23-alpine

# install gcc dependencies into alpine for CGO
RUN apk --no-cache add git ca-certificates gcc musl-dev libc-dev binutils-gold curl openssh

# install docker tools
# https://docs.docker.com/engine/install/debian/
RUN apk add --update --no-cache docker

# install linter
# binary will be $(go env GOPATH)/bin/golangci-lint
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin v1.62.2
RUN ls -la $(which golangci-lint)

# install vektra/mockery
RUN go install github.com/vektra/mockery/v2@v2.50.0

test:
  BUILD +lint
  BUILD +local-test

code:
    WORKDIR /app

    # download deps
    COPY go.mod go.sum ./
    RUN go mod download -x

    # copy in code
    COPY --dir . ./

vendor:
    FROM +code

    COPY +mock/mocks ./mocks

    RUN go mod tidy && go mod vendor
    SAVE ARTIFACT /app /files

mock:
    # copy in the necessary files that need mock generated code
    FROM +code

    # generate the mocks
    RUN mockery  --dir hash --name Hasher --keeptree --exported=true --with-expecter=true --inpackage=true --disable-version-string=true --output ./mocks/hash --case snake
    RUN mockery  --dir discovery --name Provider --keeptree --exported=true --with-expecter=true --inpackage=true --disable-version-string=true --output ./mocks/discovery --case snake

    SAVE ARTIFACT ./mocks mocks AS LOCAL mocks

lint:
    FROM +vendor

    COPY .golangci.yml ./

    # Runs golangci-lint with settings:
    RUN golangci-lint run --timeout 10m