VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMPOSE_PROJECT_NAME ?= integrationhub
COMPOSE_FILE ?= -f infrastructure/docker/compose/compose.dev.yaml
DOCKER_BUILD_ARGS ?=

.PHONY: help build test verify up down run shell logs migrate seed clean lint format security health

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}

build: ## Build all Docker images
	docker compose $(COMPOSE_FILE) build

test: ## Run all tests in Docker
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "go test ./..."

verify: build test ## Build and test (quality gate)

up: verify ## Start all services (depends on verify)
	docker compose $(COMPOSE_FILE) up -d

down: ## Stop all services
	docker compose $(COMPOSE_FILE) down

run: ## Run services in foreground
	docker compose $(COMPOSE_FILE) up

shell: ## Open shell in server container
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh

logs: ## View service logs
	docker compose $(COMPOSE_FILE) logs -f

health: ## Check service health
	docker compose $(COMPOSE_FILE) ps
	@echo "--- server healthz ---"
	@docker compose $(COMPOSE_FILE) exec -T server /bin/sh -c "wget -qO- http://localhost:8080/healthz || echo 'unhealthy'"

migrate-up: ## Run database migrations
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "go run ./cmd/migrate"

migrate-down: ## Rollback database migrations
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "go run ./cmd/migrate --down"

seed: ## Run database seeders
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "go run ./cmd/seed"

clean: ## Clean build artifacts
	docker compose $(COMPOSE_FILE) down --volumes --remove-orphans
	rm -rf cmd/server/bin web/node_modules coverage/

lint: ## Run linters
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "golangci-lint run ./..."

format: ## Format code
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "gofmt -w ."

security: ## Run security scans
	docker compose $(COMPOSE_FILE) run --rm server /bin/sh -c "gosec ./..."

migrate: migrate-up
seed-up: seed
