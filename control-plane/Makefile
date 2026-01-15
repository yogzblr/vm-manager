.PHONY: build test docker-build migrate clean run lint test-integration

BINARY_NAME=control-plane
VERSION?=1.0.0
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-ldflags "-X github.com/yourorg/control-plane/internal/version.Version=$(VERSION) \
                  -X github.com/yourorg/control-plane/internal/version.BuildDate=$(BUILD_DATE) \
                  -X github.com/yourorg/control-plane/internal/version.GitCommit=$(GIT_COMMIT)"

build:
	go build $(LDFLAGS) -o bin/$(BINARY_NAME) cmd/server/main.go

test:
	go test -v -race -cover ./...

test-integration:
	go test -v -tags=integration ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -t control-plane:$(VERSION) -f Dockerfile .

migrate:
	@echo "Running database migrations..."
	@if [ -z "$(DB_HOST)" ]; then echo "DB_HOST not set"; exit 1; fi
	@mysql -h $(DB_HOST) -u $(DB_USER) -p$(DB_PASSWORD) $(DB_NAME) < db/migrations/001_initial.sql
	@mysql -h $(DB_HOST) -u $(DB_USER) -p$(DB_PASSWORD) $(DB_NAME) < db/migrations/002_indices.sql
	@mysql -h $(DB_HOST) -u $(DB_USER) -p$(DB_PASSWORD) $(DB_NAME) < db/migrations/003_audit.sql
	@echo "Migrations complete"

migrate-down:
	@echo "Rolling back migrations..."
	@mysql -h $(DB_HOST) -u $(DB_USER) -p$(DB_PASSWORD) $(DB_NAME) -e "DROP DATABASE IF EXISTS $(DB_NAME); CREATE DATABASE $(DB_NAME);"

clean:
	rm -rf bin/ coverage.out coverage.html

run:
	go run cmd/server/main.go

run-dev:
	go run cmd/server/main.go --config config.dev.yaml

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

generate:
	go generate ./...

swagger:
	swag init -g cmd/server/main.go -o docs/
