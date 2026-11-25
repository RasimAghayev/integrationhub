# ADR-004: Persistence Strategy

## Status
Proposed

## Context
IntegrationHub needs to store configuration/metadata (ACID-friendly) and response data (schema-flexible, high throughput), plus caching and eventing.

## Decision
Polyglot persistence:

| Store | Technology | Responsibility | Rationale |
|-------|-----------|----------------|-----------|
| PostgreSQL 16 | pgx/v5 | Integrations, connections, endpoints, field mappings, executions, schedules, users, RBAC | ACID, JSONB for flexible config, relational integrity |
| MongoDB 7 | mongo-driver | Raw API responses, transformed records, audit logs, event history | Schema-flexible, high-throughput document writes |
| Redis 7 | go-redis/v9 | Rate-limit counters, sessions, hot cache, temporary data | Sub-millisecond, TTL-based, not source of truth |
| NATS JetStream | nats.go | Execution events, webhook triggers, DLQ messages | At-least-once delivery, durable streams |

## Consequences
- Two source-of-truth stores (PostgreSQL config, MongoDB data) require eventual consistency between them.
- Outbox pattern bridges PostgreSQL commits to NATS publishes (no data loss if broker is down).
- Redis is ephemeral; loss is acceptable (rebuildable from source stores).
- Operational complexity of 4 stores vs. single-store simplicity; justified by access patterns.

## Alternatives Considered
- Single PostgreSQL with JSONB: rejected — high-throughput unstructured response writes would contend with ACID config workloads.
- Single MongoDB: rejected — config metadata needs ACID transactions and relational integrity.
