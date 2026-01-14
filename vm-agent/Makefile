.PHONY: build test docker-build clean build-linux build-windows build-darwin build-all test-coverage lint install

BINARY_NAME=vm-agent
VERSION?=1.0.0
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X github.com/yourorg/vm-agent/internal/version.Version=$(VERSION) \
                  -X github.com/yourorg/vm-agent/internal/version.BuildDate=$(BUILD_DATE) \
                  -X github.com/yourorg/vm-agent/internal/version.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) cmd/agent/main.go

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-linux-amd64 cmd/agent/main.go

build-windows:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-windows-amd64.exe cmd/agent/main.go

build-darwin:
	CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_NAME)-darwin-amd64 cmd/agent/main.go

build-all: build-linux build-windows build-darwin

test:
	go test -v -race -cover ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

docker-build:
	docker build -t vm-agent:$(VERSION) -f Dockerfile .

clean:
	rm -rf bin/ coverage.out coverage.html

install:
	go install $(LDFLAGS) ./cmd/agent/
