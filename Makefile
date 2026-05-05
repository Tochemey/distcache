IMAGE         ?= distcache-tools:latest
GO_TEST_FLAGS ?= -race -coverprofile=coverage.out -covermode=atomic -coverpkg=./...

DOCKER_RUN := docker run --rm \
	-v $(CURDIR):/app \
	-w /app \
	$(IMAGE)

.PHONY: image vendor mock lint test clean

image:
	docker build -t $(IMAGE) -f Dockerfile .

vendor: image
	$(DOCKER_RUN) sh -c 'go mod tidy && go mod vendor'

mock: image
	$(DOCKER_RUN) sh -c '\
		mockery --dir hash --name Hasher --keeptree --exported=true --with-expecter=true --inpackage=true --disable-version-string=true --output ./mocks/hash --case snake && \
		mockery --dir discovery --name Provider --keeptree --exported=true --with-expecter=true --inpackage=true --disable-version-string=true --output ./mocks/discovery --case snake'

lint: image
	$(DOCKER_RUN) golangci-lint run --timeout 10m --config .golangci.yml

test: image
	$(DOCKER_RUN) go test -mod=vendor ./... $(GO_TEST_FLAGS)

clean:
	-docker rmi $(IMAGE)
