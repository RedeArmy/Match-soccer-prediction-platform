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

.PHONY: build run run-worker test test-integration test-cover check-coverage check-coverage-service test-migrate-roundtrip lint vuln clean docker-up docker-down docker-logs migrate migrate-fresh migrate-fresh-seed schema-dump dev hooks check-env swagger-gen swagger-clean validate-params bench bench-save bench-compare load-test help

## build: Compile all binaries into ./bin
build:
	@mkdir -p $(BINARY_DIR)
	go build -ldflags="-s -w" -o $(API_BIN)     ./cmd/api
	go build -ldflags="-s -w" -o $(MIGRATE_BIN) ./cmd/migrate
	go build -ldflags="-s -w" -o $(WORKER_BIN)  ./cmd/worker

## run: Run the API server with local development settings
##      Requires: `make docker-up` to be running first.
run:
	WCQ_ENVIRONMENT=dev \
	WCQ_LOGGER_ENCODING=console \
	WCQ_DATABASE_DSN=postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable \
	go run ./cmd/api

## run-worker: Run the background worker with local development settings
##             Requires: `make docker-up` to be running first (Redis required).
run-worker:
	WCQ_ENVIRONMENT=dev \
	WCQ_LOGGER_ENCODING=console \
	WCQ_DATABASE_DSN=postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable \
	WCQ_EVENTBUS_DRIVER=redis \
	go run ./cmd/worker

## test: Run the full test suite with race detection enabled
##       The -count=1 flag disables the test cache, ensuring each run is fresh.
##       The 90s timeout accommodates internal/infrastructure/database, which
##       starts multiple testcontainer instances sequentially (one per test)
##       and can legitimately require ~70s on a warm CI runner.
test:
	go test -race -count=1 -timeout=90s ./...

## test-integration: Run end-to-end tests that require a live Docker daemon.
##                   These tests are excluded from the standard test / test-cover
##                   targets to avoid adding Docker pull latency to the unit-test
##                   budget. Requires Docker to be running locally.
test-integration:
	go test -race -count=1 -timeout=120s -tags integration ./internal/api/...

## test-migrate-roundtrip: Validate every down migration by running the full
##                         up → down → up cycle against a real Postgres container.
##                         Fails if any .down.sql file is syntactically broken,
##                         references a non-existent object, or leaves the schema
##                         in a state that prevents re-migration. Requires Docker.
test-migrate-roundtrip:
	go test -v -count=1 -timeout=5m -run TestMigrateRoundtrip \
		./internal/infrastructure/database/...

## test-cover: Run the full test suite and emit a coverage profile for SonarCloud
##             Output: coverage.out (Go native format, read directly by SonarCloud)
##             The -covermode=atomic flag is required when -race is enabled; it
##             uses atomic operations to update counters safely across goroutines.
##             The -coverpkg=./... flag instruments every package in the module,
##             not just the one under test, so cross-package helpers such as
##             internal/testutil are counted when called from other packages' tests.
test-cover:
	rm -f coverage*.out
	go test -race -count=1 -timeout=120s -short \
		-coverprofile=coverage.out \
		-covermode=atomic \
		-coverpkg=./... \
		./...

## check-coverage: Fail if total test coverage is below MIN_COVERAGE (default: 70%).
##                 Reads coverage.out produced by `make test-cover`.
##                 Acts as a local fallback for the SonarCloud quality gate so a
##                 coverage drop is caught even when sonarcloud.io is unavailable.
##                 Override: make check-coverage MIN_COVERAGE=80
MIN_COVERAGE ?= 70
check-coverage:
	@[ -f coverage.out ] || { \
		echo "coverage.out not found — run 'make test-cover' first"; exit 1; \
	}
	@COVERAGE=$$(go tool cover -func coverage.out \
		| grep '^total:' \
		| awk '{print $$NF}' \
		| tr -d '%'); \
	echo "Coverage: $${COVERAGE}%  (minimum: $(MIN_COVERAGE)%)"; \
	awk -v got="$${COVERAGE}" -v min="$(MIN_COVERAGE)" \
		'BEGIN { if (got+0 < min+0) { \
			print "FAIL: " got "% is below minimum " min "%"; exit 1 \
		} else { print "PASS" } }'

## check-coverage-service: Fail if internal/service/ unit-test coverage is below MIN_SVC_COVERAGE (default: 80%).
##                          Runs tests directly — does NOT require a pre-existing coverage.out.
##                          internal/repository/ is excluded: its tests are integration-only (testcontainers)
##                          and measured by the test-integration job, not unit-test jobs.
##                          Override: make check-coverage-service MIN_SVC_COVERAGE=85
MIN_SVC_COVERAGE ?= 80
check-coverage-service:
	@go test -count=1 -short \
		-coverprofile=/tmp/cov_service.out \
		-covermode=atomic \
		./internal/service/... 2>/dev/null | tail -1
	@COVERAGE=$$(go tool cover -func=/tmp/cov_service.out \
		| grep '^total:' \
		| awk '{print $$NF}' \
		| tr -d '%'); \
	echo "internal/service coverage: $${COVERAGE}%  (minimum: $(MIN_SVC_COVERAGE)%)"; \
	awk -v got="$${COVERAGE}" -v min="$(MIN_SVC_COVERAGE)" \
		'BEGIN { if (got+0 < min+0) { \
			print "FAIL: " got "% is below minimum " min "%"; exit 1 \
		} else { print "PASS" } }'

## lint: Run golangci-lint across the entire module
##       Install golangci-lint: https://golangci-lint.run/usage/install/
lint:
	golangci-lint run ./...

## vuln: Scan the module for known CVEs using govulncheck (pinned version).
##       Exits non-zero on any finding. Run before opening a PR or releasing.
vuln:
	go install golang.org/x/vuln/cmd/govulncheck@v1.1.3
	govulncheck ./...

## clean: Remove compiled binaries and coverage artifacts
clean:
	rm -rf $(BINARY_DIR)
	rm -f *.out

## docker-up: Start Postgres and Redis containers in the background
docker-up:
	docker compose up -d

## docker-down: Stop and remove all containers (data volumes are preserved)
docker-down:
	docker compose down

## docker-logs: Tail logs from all running compose services
docker-logs:
	docker compose logs -f

## dev: Start infrastructure, install git hooks, and apply migrations (full local setup)
##      Requires Docker to be running. Blocks until Postgres is healthy, then migrates.
dev: docker-up hooks
	@echo "Waiting for Postgres to be healthy..."
	@docker compose exec postgres sh -c 'until pg_isready -U $${POSTGRES_USER:-quiniela} -d $${POSTGRES_DB:-quiniela}; do sleep 1; done'
	$(MAKE) migrate

## hooks: Install project Git hooks (run once after cloning).
##        The pre-commit hook regenerates Swagger docs and (when gitleaks is
##        installed) scans staged files for secrets before every commit.
##        Install gitleaks: go install github.com/zricethezav/gitleaks/v8/cmd/gitleaks@latest
hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks installed from .githooks/"

## check-env: Verify that .env is not tracked by Git and print a credential reminder.
##            Safe to run at any time — advisory only, exits 0.
check-env:
	@if git ls-files --error-unmatch .env >/dev/null 2>&1; then \
		echo "ERROR: .env is tracked by Git."; \
		echo "       Untrack it immediately: git rm --cached .env && git commit -m 'chore: untrack .env'"; \
		exit 1; \
	fi
	@echo "OK  .env is not tracked by Git"
	@if [ ! -f .env ]; then \
		echo "INFO .env not found — copy from template: cp .env.example .env"; \
	else \
		echo "OK  .env exists locally"; \
		echo ""; \
		echo "REMINDER: .env is for local development only."; \
		echo "          Never copy production secrets (Clerk, PayPal, Resend, payout key) into .env."; \
		echo "          Use 'fly secrets set ...' to inject secrets in production."; \
	fi

## migrate: Apply pending database schema migrations
migrate:
	go run ./cmd/migrate

## migrate-fresh: Bootstrap a new environment from the consolidated baseline DDL.
##                Applies migrations/baseline/schema.sql directly (skips running
##                all 67+ migrations sequentially) and marks every versioned
##                migration as applied so golang-migrate sees no pending changes.
##                ONLY for new environments — never run against a DB that already
##                has data. Regenerate the baseline with `make schema-dump` after
##                adding new migrations.
migrate-fresh:
	go run ./cmd/migrate --fresh

## migrate-fresh-seed: Same as migrate-fresh but also inserts development fixtures.
migrate-fresh-seed:
	go run ./cmd/migrate --fresh --seed

## schema-dump: Regenerate migrations/baseline/schema.sql from the running database.
##              Run this after adding new migrations so migrate-fresh stays current.
##              Requires pg_dump to be installed and the database to be running.
schema-dump:
	@echo "Dumping schema from running database..."
	pg_dump \
		--schema-only \
		--no-owner \
		--no-acl \
		--no-comments \
		"postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable" \
	| grep -v "^--" \
	| sed '/^$$/N;/^\n$$/d' \
	> migrations/baseline/schema.sql
	@echo "-- AUTO-GENERATED by schema-dump — do not edit by hand." | cat - migrations/baseline/schema.sql > /tmp/schema_tmp.sql && mv /tmp/schema_tmp.sql migrations/baseline/schema.sql
	@echo "Schema dumped to migrations/baseline/schema.sql"

## validate-params: Validate system_params table synchronization with constants.go
##                  Requires: Database to be running (make docker-up) and migrated (make migrate).
##                  Verifies that every ParamKey* constant has a matching row in system_params
##                  with correct type, category, and description.
validate-params:
	@DATABASE_URL="postgres://quiniela:quiniela@localhost:5432/quiniela?sslmode=disable" \
	go run ./cmd/validate-params

## swagger-gen: Generate OpenAPI spec and Swagger UI assets from handler annotations.
##              Install the CLI once with: go install github.com/swaggo/swag/cmd/swag
swagger-gen:
	swag init \
		--generalInfo cmd/api/main.go \
		--output docs \
		--outputTypes go,json,yaml \
		--parseDependency \
		--parseInternal \
		--dir .

## swagger-clean: Remove all generated Swagger docs (re-run swagger-gen to rebuild)
swagger-clean:
	rm -rf docs/*.go docs/*.json docs/*.yaml

## bench: Run all benchmarks once and print results to stdout.
##        Use -count=N to increase run count for more stable measurements.
##        Example: make bench COUNT=6
COUNT ?= 1
bench:
	go test -bench=. -benchmem -benchtime=1x -count=$(COUNT) -run='^$$' \
		./internal/notification/... \
		./internal/service/... \
		./internal/api/...

## bench-save: Run benchmarks and save results to .bench/current.txt.
##             Commit the output as .bench/baseline.txt to establish a new baseline.
bench-save:
	@mkdir -p .bench
	go test -bench=. -benchmem -count=6 -run='^$$' \
		./internal/notification/... \
		./internal/service/... \
		./internal/api/... \
	| tee .bench/current.txt
	@echo ""
	@echo "Results saved to .bench/current.txt"
	@echo "To update the baseline: cp .bench/current.txt .bench/baseline.txt"

## bench-compare: Run benchmarks and diff against .bench/baseline.txt using benchstat.
##                Requires a baseline: run 'make bench-save' and commit .bench/baseline.txt first.
bench-compare:
	@if [ ! -f .bench/baseline.txt ]; then \
		echo "No baseline found at .bench/baseline.txt. Run 'make bench-save' first."; \
		exit 1; \
	fi
	@mkdir -p .bench
	go test -bench=. -benchmem -count=6 -run='^$$' \
		./internal/notification/... \
		./internal/service/... \
		./internal/api/... \
	> .bench/current.txt
	go run golang.org/x/perf/cmd/benchstat .bench/baseline.txt .bench/current.txt

## load-test: Fire an HTTP load test against a running server (manual use only — not in CI).
##            Requires hey: go install github.com/rakyll/hey@latest
##            Requires the server to be running: make run (in another terminal)
##
##            Defaults exercise /health/ready at fly.toml soft_limit concurrency (200 req, 200 conc).
##            Override any variable:
##              make load-test LOAD_TEST_N=1000 LOAD_TEST_C=50 LOAD_TEST_PATH=/api/v1/matches
##
##            Authenticated endpoints: set LOAD_TEST_AUTH to a valid Bearer token.
##              make load-test LOAD_TEST_PATH=/api/v1/predictions/me LOAD_TEST_AUTH="Bearer <token>"
LOAD_TEST_ADDR ?= http://localhost:8080
LOAD_TEST_PATH ?= /health/ready
LOAD_TEST_N    ?= 200
LOAD_TEST_C    ?= 200
LOAD_TEST_AUTH ?=
load-test:
	@command -v hey >/dev/null 2>&1 || { echo "hey not found. Install: go install github.com/rakyll/hey@latest"; exit 1; }
	@echo "→ Load test: $(LOAD_TEST_ADDR)$(LOAD_TEST_PATH) — $(LOAD_TEST_N) requests, $(LOAD_TEST_C) concurrent"
	@if [ -n "$(LOAD_TEST_AUTH)" ]; then \
		hey -n $(LOAD_TEST_N) -c $(LOAD_TEST_C) -H "Authorization: $(LOAD_TEST_AUTH)" "$(LOAD_TEST_ADDR)$(LOAD_TEST_PATH)"; \
	else \
		hey -n $(LOAD_TEST_N) -c $(LOAD_TEST_C) "$(LOAD_TEST_ADDR)$(LOAD_TEST_PATH)"; \
	fi

## help: Display this help message
help:
	@echo ""
	@echo "Usage: make [target]"
	@echo ""
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/^## /  /'
	@echo ""
