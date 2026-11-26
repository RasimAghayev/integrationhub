VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMPOSE_FILE := infrastructure/docker/compose/compose.dev.yaml
COMPOSE := docker compose -f $(COMPOSE_FILE)
GO_IMAGE := golang:1.26
GO_RUN := docker run --rm -v "$(CURDIR)":/app -w /app -e GOTOOLCHAIN=local -e GOFLAGS=-mod=mod $(GO_IMAGE)

.PHONY: help build test test-integration verify up down run shell logs loadtest clean lint format security

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build all Docker images
	$(COMPOSE) build

test: ## Run all tests
	$(GO_RUN) go test ./... -cover

test-integration: ## Run integration tests (testcontainers, needs Docker)
	$(GO_RUN) go test -tags integration ./... -cover

verify: build test ## Build and test (quality gate)

up: build ## Start all services (migrations run automatically at startup)
	$(COMPOSE) up -d

down: ## Stop all services
	$(COMPOSE) down

run: ## Run services in foreground
	$(COMPOSE) up

shell: ## Open shell in server container
	$(COMPOSE) run --rm server /bin/sh

logs: ## View service logs
	$(COMPOSE) logs -f

loadtest: ## Run the HTTP load generator (make loadtest ARGS="-integration <id> -workers 20")
	$(GO_RUN) go run ./cmd/loadtest $(ARGS)

clean: ## Clean build artifacts
	$(COMPOSE) down --volumes --remove-orphans
	rm -rf cmd/server/bin coverage/

lint: ## Run linters
	$(GO_RUN) go vet ./...
	$(GO_RUN) sh -c "test -z \"$$(gofmt -l .)\""

format: ## Format code
	$(GO_RUN) gofmt -w .

security: ## Run security scans
	$(GO_RUN) sh -c "go run github.com/securego/gosec/v2/cmd/gosec@latest ./... || true"