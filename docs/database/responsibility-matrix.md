# Database Responsibility Matrix

| Data Type | Store | Rationale | TTL / Retention | Source of Truth |
|-----------|-------|-----------|-----------------|-----------------|
| Integrations | PostgreSQL | ACID config, relational integrity | Indefinite | Yes |
| Connections | PostgreSQL | ACID config, encrypted credentials | Indefinite | Yes |
| Endpoints | PostgreSQL | Belongs to integration; relational | Indefinite | Yes |
| FieldMappings | PostgreSQL | Config mapping, relational | Indefinite | Yes |
| Executions (metadata) | PostgreSQL | Execution history, ACID | 90 days (configurable) | Yes |
| Schedules | PostgreSQL | Cron config, ACID | Indefinite | Yes |
| Users & RBAC | PostgreSQL | Identity, ACID | Indefinite | Yes |
| Raw API responses | MongoDB | Schema-flexible, high throughput | 30 days (configurable) | Yes |
| Transformed records | MongoDB | Schema-flexible documents | 90 days (configurable) | Yes |
| Audit logs | MongoDB | Immutable append-only, flexible | 365 days | Yes |
| Rate-limit counters | Redis | Sub-millisecond, TTL-based | 60 seconds | No (rebuildable) |
| Sessions | Redis | TTL-based, ephemeral | 24 hours | No |
| Hot cache | Redis | Frequently accessed config | 5 minutes | No |
| Execution events | NATS JetStream | At-least-once, durable streams | 7 days | No (event log) |
| Webhook triggers | NATS JetStream | Incoming webhook ingestion | 7 days | No |
| DLQ messages | NATS JetStream | Failed event processing | 30 days | No |

## Cache Invalidation & TTL Policy
- Rate-limit counters: sliding window, 60s TTL.
- Sessions: 24h absolute TTL, renewed on activity.
- Hot cache: 5m TTL, invalidated on config write.
- All Redis data is rebuildable from PostgreSQL/MongoDB; loss is acceptable.

## Cross-Store Consistency
- PostgreSQL → NATS: Transactional Outbox. On each PostgreSQL commit that creates/modifies an execution, an outbox row is written in the same transaction. A separate publisher reads outbox rows and publishes to NATS, then marks them sent.
- PostgreSQL → MongoDB: Best-effort. Execution metadata committed first; response data written to MongoDB after. MongoDB write failure is retried; metadata remains consistent.
