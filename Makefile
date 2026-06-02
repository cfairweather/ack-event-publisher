IMG ?= ghcr.io/aws-controllers-k8s/ack-event-publisher:latest

GIT_VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
GIT_COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE  := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.GitVersion=$(GIT_VERSION) \
           -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.GitCommit=$(GIT_COMMIT) \
           -X github.com/aws-controllers-k8s/ack-event-publisher/pkg/version.BuildDate=$(BUILD_DATE)

.PHONY: build test tidy vet lint docker-build helm-lint

build:
	go build -ldflags "$(LDFLAGS)" -o bin/ack-event-publisher ./cmd/...

test:
	go test ./... -v

tidy:
	go mod tidy

vet:
	go vet ./...

lint: vet
	@which golangci-lint > /dev/null || (echo "golangci-lint not found: https://golangci-lint.run/usage/install/" && exit 1)
	golangci-lint run ./...

docker-build:
	docker build \
		--build-arg GIT_VERSION=$(GIT_VERSION) \
		--build-arg GIT_COMMIT=$(GIT_COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMG) .

helm-lint:
	helm lint ./helm
