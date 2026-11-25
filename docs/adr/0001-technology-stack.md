# ADR-001: Technology Stack

## Status
Proposed

## Context
IntegrationHub needs a technology stack for a Go-based API integration platform connecting to 3rd party APIs (REST, webhooks, GraphQL) with a web admin UI, field mapping, scheduling, and event-driven execution triggers.

## Decision
- Language: Go 1.22+ — concurrency (goroutines), performance, single binary deployment
- HTTP Router: Chi (chi/v5) — lightweight, composable middleware, stdlib-compatible
- OpenAPI: swaggo — standard OpenAPI 3.0 generation
- Database (Config/Metadata): PostgreSQL 16 (pgx/v5) — ACID, JSONB for flexible config
- Database (Response Data): MongoDB 7 (mongo-driver) — schema-flexible, high throughput
- Cache: Redis 7 (go-redis/v9) — rate limiting, sessions, hot data
- Message Queue: NATS JetStream (nats.go) — event-driven, at-least-once delivery
- Scheduler: robfig/cron/v3 — cron expression support
- Auth: golang-jwt/v5 + oauth2 + casbin — JWT + OAuth2 + RBAC authorization
- Transform: gval (jsonpath) + text/template + gopher-lua — visual + template + code transforms
- UI: React + TypeScript + Tailwind + shadcn/ui — modern component-based admin UI
- Build: Make + Docker multi-stage — standard
- Testing: testify + testcontainers-go + gomock — unit + integration + mock
- CI/CD: GitHub Actions + golangci-lint — quality gates
- Observability: OpenTelemetry + Prometheus + Loki — traces, metrics, logs

## Consequences
- Single binary Go server simplifies deployment and operations
- Polyglot persistence (PostgreSQL + MongoDB) matches data access patterns (ACID config vs schema-flexible responses)
- NATS JetStream enables event-driven triggers and reliable execution delivery
- Lua scripting adds transform flexibility but introduces a dependency and sandbox considerations
- Go 1.22 is the minimum supported version; upgraded per standing decision 1 (latest stable)

## Alternatives Considered
- Gin vs Chi: Chi chosen for stdlib compatibility and lighter weight
- Kafka vs NATS: NATS chosen for simpler operations and JetStream at-least-once semantics
- MySQL vs PostgreSQL: PostgreSQL chosen for JSONB and ACID guarantees
