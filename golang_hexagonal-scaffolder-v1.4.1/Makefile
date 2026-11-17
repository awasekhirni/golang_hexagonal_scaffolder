# =============================================================================
# hexagonal-scaffolder — root Makefile
# Cross-project tasks for the generator and generated-api Go modules.
# Both modules are independent: each has its own go.mod. Targets that touch
# both cd into each module in turn.
# =============================================================================

SHELL := /bin/bash
.DEFAULT_GOAL := help

VERSION := $(shell cat VERSION 2>/dev/null || echo 0.0.0)
ROOT    := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

GOFLAGS  := -trimpath
TESTFLAGS := -race -count=1

# Color helpers (only when stdout is a tty).
ifeq ($(shell test -t 1 && echo yes),yes)
  BOLD := \033[1m
  DIM  := \033[2m
  END  := \033[0m
else
  BOLD :=
  DIM  :=
  END  :=
endif

.PHONY: help
help: ## Show this help.
	@printf "$(BOLD)hexagonal-scaffolder$(END) v$(VERSION)\n\n"
	@awk 'BEGIN {FS = ":.*##"; printf "Usage:\n  make $(BOLD)<target>$(END) [VAR=value ...]\n\nTargets:\n"} \
	  /^[a-zA-Z_-]+:.*?##/ { printf "  $(DIM)%-18s$(END) %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

# -----------------------------------------------------------------------------
# Build
# -----------------------------------------------------------------------------
.PHONY: build
build: build-generator build-api ## Build both modules' binaries.

.PHONY: build-generator
build-generator: ## Build the generator binary into bin/generator.
	@printf "$(BOLD)→ building generator$(END)\n"
	@cd generator && go build $(GOFLAGS) -o ../bin/generator ./cmd/generator

.PHONY: build-api
build-api: ## Build the generated-api binary into bin/api.
	@printf "$(BOLD)→ building generated-api$(END)\n"
	@cd generated-api && go build $(GOFLAGS) -o ../bin/api ./cmd/api

# -----------------------------------------------------------------------------
# Test / vet / cover
# -----------------------------------------------------------------------------
.PHONY: vet
vet: ## go vet both modules.
	@cd generator     && go vet ./...
	@cd generated-api && go vet ./...

.PHONY: test
test: ## go test both modules (race + count=1). Never swallows compile errors.
	@printf "$(BOLD)→ test generator$(END)\n"
	@cd generator     && go test $(TESTFLAGS) ./...
	@printf "$(BOLD)→ test generated-api$(END)\n"
	@cd generated-api && go test $(TESTFLAGS) ./...

.PHONY: cover
cover: ## Write coverage HTML for both modules into ./coverage/*.html.
	@mkdir -p coverage
	@cd generator     && go test -coverprofile=$(ROOT)coverage/generator.out     ./... && go tool cover -html=$(ROOT)coverage/generator.out     -o $(ROOT)coverage/generator.html
	@cd generated-api && go test -coverprofile=$(ROOT)coverage/generated-api.out ./... && go tool cover -html=$(ROOT)coverage/generated-api.out -o $(ROOT)coverage/generated-api.html
	@printf "Coverage HTML: $(BOLD)coverage/generator.html$(END), $(BOLD)coverage/generated-api.html$(END)\n"

.PHONY: tidy
tidy: ## go mod tidy both modules.
	@cd generator     && go mod tidy
	@cd generated-api && go mod tidy

.PHONY: fmt
fmt: ## gofmt + goimports both modules.
	@cd generator     && gofmt -s -w . && goimports -w . 2>/dev/null || true
	@cd generated-api && gofmt -s -w . && goimports -w . 2>/dev/null || true

.PHONY: lint
lint: ## go vet + (optional) golangci-lint.
	@$(MAKE) vet
	@command -v golangci-lint >/dev/null && { cd generator     && golangci-lint run ./...; cd ../generated-api && golangci-lint run ./...; } || printf "$(DIM)golangci-lint not installed; skipped$(END)\n"

# -----------------------------------------------------------------------------
# Integration / run
# -----------------------------------------------------------------------------
.PHONY: run-generator
run-generator: ## Run the generator CLI with --help.
	@cd generator && go run ./cmd/generator --help

.PHONY: run-api
run-api: ## Run the generated-api server (needs postgres up).
	@cd generated-api && go run ./cmd/api

# NOTE: v1.4.0 — `run-integration` no longer swallows compile errors.
# Earlier versions piped `2>/dev/null`, which hid real failures in CI.
.PHONY: run-integration
run-integration: build ## Bring up postgres, run the generator against it, smoke-test the API.
	@printf "$(BOLD)→ integration: start postgres$(END)\n"
	@docker compose up -d postgres
	@printf "$(BOLD)→ integration: wait for postgres healthy$(END)\n"
	@timeout 60 bash -c 'until docker compose ps postgres | grep -q healthy; do sleep 2; done'
	@printf "$(BOLD)→ integration: run generator$(END)\n"
	@./bin/generator \
	  --db-host localhost --db-port 5432 \
	  --db-user hexapi --db-password "$$(grep ^POSTGRES_PASSWORD= .env | cut -d= -f2-)" \
	  --db-name hexapi_db --out ./out-integration --module github.com/example/integration-out
	@printf "$(BOLD)→ integration: smoke test API$(END)\n"
	@./bin/api &
	@API_PID=$$!; sleep 3; \
	  curl -fsS http://localhost:8080/health/ready; \
	  kill $$API_PID

# -----------------------------------------------------------------------------
# Docker
# -----------------------------------------------------------------------------
.PHONY: up
up: ## docker compose up --build (postgres + api).
	@test -f .env || { printf "$(BOLD).env missing. Run: cp .env.example .env$(END)\n"; exit 1; }
	@docker compose up --build

.PHONY: down
down: ## docker compose down -v.
	@docker compose down -v

.PHONY: migrate
migrate: ## Apply SQL migrations to the running postgres container.
	@docker compose exec -T postgres psql -U hexapi -d hexapi_db < generated-api/resources/migrations/0001_init.sql

.PHONY: smoke
smoke: ## curl /health/ready and /users against the running api.
	@curl -fsS http://localhost:8080/health/ready && printf "\n"
	@curl -fsS http://localhost:8080/users && printf "\n"

# -----------------------------------------------------------------------------
# Packaging
# -----------------------------------------------------------------------------
.PHONY: zip
zip: ## Produce hexagonal-scaffolder-v$(VERSION).zip (excludes .git, build artifacts).
	@printf "$(BOLD)→ packaging v$(VERSION)$(END)\n"
	@cd $(ROOT).. && rm -f hexagonal-scaffolder-v$(VERSION).zip && \
	  zip -qr hexagonal-scaffolder-v$(VERSION).zip hexagonal-scaffolder-v$(VERSION) \
	    -x '*/.git/*' '*/bin/*' '*/coverage/*' '*/out/*' '*/out-integration/*' '*.zip'
	@printf "Wrote: $(BOLD)../hexagonal-scaffolder-v$(VERSION).zip$(END)\n"

.PHONY: clean
clean: ## Remove build artifacts.
	@rm -rf bin coverage out out-integration
