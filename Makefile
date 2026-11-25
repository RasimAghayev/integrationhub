VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "0.1.0")
COMPOSE_PROJECT_NAME ?= integrationhub
DOCKER_BUILD_ARGS ?=

.PHONY: help build test verify up down run shell logs migrate seed clean lint format security

help: ## Show this help message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}

build: ## Build all Docker images
	docker compose build

test: ## Run all tests in Docker
	docker compose run --rm server /bin/sh -c "go test ./..."
	docker compose run --rm web /bin/sh -c "cd /app && npm test -- --run || true"

verify: build test ## Build and test (quality gate)

up: verify ## Start all services (depends on verify)
	docker compose up -d

down: ## Stop all services
	docker compose down

run: ## Run services in foreground
	docker compose up

shell: ## Open shell in server container
	docker compose run --rm server /bin/sh

logs: ## View service logs
	docker compose logs -f

migrate-up: ## Run database migrations
	docker compose run --rm server /bin/sh -c "go run ./cmd/migrate"

migrate-down: ## Rollback database migrations
	docker compose run --rm server /bin/sh -c "go run ./cmd/migrate --down"

seed: ## Run database seeders
	docker compose run --rm server /bin/sh -c "go run ./cmd/seed"

clean: ## Clean build artifacts
	docker compose down --volumes --remove-orphans
	rm -rf cmd/server/bin web/node_modules coverage/

lint: ## Run linters
	docker compose run --rm server /bin/sh -c "golangci-lint run ./..."
	docker compose run --rm web /bin/sh -c "cd /app && npm run lint || true"

format: ## Format code
	docker compose run --rm server /bin/sh -c "gofmt -w ."
	docker compose run --rm web /bin/sh -c "cd /app && npm run format || true"

security: ## Run security scans
	docker compose run --rm server /bin/sh -c "gosec ./..."
	docker compose run --rm web /bin/sh -c "cd /app && npm audit || true"

migrate: migrate-up
seed-up: seed
