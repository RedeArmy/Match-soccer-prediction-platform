# ══════════════════════════════════════════════════════════════════════════════
# World Cup Quiniela — Developer Makefile
#
# Run `make help` to see all available targets with descriptions.
# ══════════════════════════════════════════════════════════════════════════════

BINARY_DIR  := bin
API_BIN     := $(BINARY_DIR)/api
MIGRATE_BIN := $(BINARY_DIR)/migrate
WORKER_BIN  := $(BINARY_DIR)/worker

# Default target: build all binaries.
.DEFAULT_GOAL := build

.PHONY: build run test test-cover lint swagger-gen clean docker-up docker-down migrate help

## build: Compile all binaries into ./bin
build:
	@mkdir -p $(BINARY_DIR)
	go build -ldflags="-s -w" -o $(API_BIN)     ./cmd/api
	go build -ldflags="-s -w" -o $(MIGRATE_BIN) ./cmd/migrate
	go build -ldflags="-s -w" -o $(WORKER_BIN)  ./cmd/worker

## run: Run the API server with local development settings
##      Requires: `make docker-up` to be running first.
run:
	WCQ_JWT_SECRET=devsecret \
	WCQ_LOGGER_ENCODING=console \
	WCQ_DATABASE_DSN=postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable \
	go run ./cmd/api

## test: Run the full test suite with race detection enabled
##       The -count=1 flag disables the test cache, ensuring each run is fresh.
test:
	go test -race -count=1 -timeout=60s ./...

## test-cover: Run the full test suite and emit a coverage profile for SonarCloud
##             Output: coverage.out (Go native format, read directly by SonarCloud)
##             The -covermode=atomic flag is required when -race is enabled; it
##             uses atomic operations to update counters safely across goroutines.
##             The -coverpkg=./... flag instruments every package in the module,
##             not just the one under test, so cross-package helpers such as
##             internal/testutil are counted when called from other packages' tests.
test-cover:
	go test -race -count=1 -timeout=60s \
		-coverprofile=coverage.out \
		-covermode=atomic \
		-coverpkg=./... \
		./...

## lint: Run golangci-lint across the entire module
##       Install golangci-lint: https://golangci-lint.run/usage/install/
lint:
	golangci-lint run ./...

## swagger-gen: Regenerate OpenAPI spec and Swagger UI assets from annotations
##              Requires swag CLI: go install github.com/swaggo/swag/cmd/swag@latest
swagger-gen:
	swag init -g cmd/api/main.go -o docs/

## clean: Remove compiled binaries
clean:
	rm -rf $(BINARY_DIR)

## docker-up: Start Postgres and Redis containers in the background
docker-up:
	docker compose up -d

## docker-down: Stop and remove all containers (data volumes are preserved)
docker-down:
	docker compose down

## migrate: Apply pending database schema migrations
migrate:
	go run ./cmd/migrate

## help: Display this help message
help:
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
	@echo ""
