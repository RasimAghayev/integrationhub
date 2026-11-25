# Domain Model — Bounded Contexts

IntegrationHub follows Domain-Driven Design with Clean Architecture. The system is split into bounded contexts, each owning its aggregates, entities, and invariants.

## 1. Integration Context (Aggregate Root: Integration)

### Integration
The Integration is the aggregate root. It ties together a Connection, one or more Endpoints, FieldMappings, a Schedule, a TransformConfig, and a RetryPolicy.

Invariants:
- An Integration has exactly one Connection.
- An Integration has at least one Endpoint.
- Schedule is optional; when absent, the integration runs only on-demand or via webhook/event trigger.
- Status transitions: Active → Paused → Error (and reverse).

### Connection (within Integration)
Represents 3rd party connectivity configuration. Owned by Integration; cannot outlive it.

Invariants:
- BaseURL must be a valid absolute URL.
- AuthConfig is encrypted at rest (AES-256-GCM); plaintext never persisted.
- RateLimit requests-per-second must be > 0.
- Deleting a Connection with active Integrations is forbidden.

### Endpoint
Represents a single 3rd party API endpoint to call.

Invariants:
- Method ∈ {GET, POST, PUT, PATCH, DELETE}.
- Path must start with `/`.
- BodyTemplate is a Go text/template; must render without error given sample input.
- Pagination config is optional; when present, one of offset/cursor/page strategy must be fully specified.

### FieldMapping
Maps a 3rd party response JSONPath to an internal target field.

Invariants:
- SourcePath is a valid JSONPath expression.
- Required=true implies a Default must exist OR the mapping must always find a value.
- Transform must be one of: none, uppercase, lowercase, format_date, compute, lookup, regex.

### Execution
Represents a single run of an Integration/Endpoint.

Invariants:
- Status ∈ {Pending, Running, Success, Failed, Retrying}.
- Duration = CompletedAt − StartedAt when CompletedAt is set.
- RecordsProcessed + RecordsFailed ≤ total records in the response batch.

### Schedule
Cron expression or trigger definition.

Invariants:
- Cron expression must be parseable by robfig/cron/v3.
- Timezone must be a valid IANA zone.

### TransformConfig
Defines how response data is transformed before storage.

Invariants:
- Type ∈ {Visual, Template, Code}.
- Visual: VisualMappings must be non-empty.
- Template: Template string must be a valid Go template.
- Code: Code + Language (lua) must be present; script must pass sandbox validation.

## 2. Connection Context

Standalone concern for managing 3rd party credentials independent of a specific Integration lifecycle.

Aggregates: Connection

Entities:
- Connection (id, name, base_url, auth_type, auth_config_encrypted, headers, rate_limit, timeout, tls_config, created_at, updated_at)

Auth types: APIKey, OAuth2, JWT, Basic, Custom.

Invariants:
- OAuth2 connections store access_token, refresh_token, expires_at encrypted.
- Token auto-refresh must trigger before expiry (configurable skew).
- Connection test hits base_url/health and returns status + latency + error.

## 3. Mapping Context

Owns FieldMapping lifecycle and the JSONPath engine.

Aggregates: FieldMapping

Invariants:
- JSONPath extraction supports: $, ., [], *, .., filters (?[@.x==1]).
- Missing required fields produce clear errors (not silent defaults).
- Mapping application is idempotent: same input → same output.

## 4. Execution Context

Owns the execution engine: worker pool, retry, circuit breaker, DLQ.

Aggregates: Execution

Entities:
- Execution, ExecutionError

Invariants:
- Worker pool concurrency is configurable (default 10).
- Per-integration concurrency limit enforced via token bucket.
- Circuit breaker per connection: closed → open → half-open.
- DLQ after max retries (default 3).
- Retryable: 5xx, timeout, connection errors. Non-retryable: 4xx except 429.

## 5. Transform Context

Owns transform rules and the transform engine.

Aggregates: TransformConfig

Invariants:
- Lua scripts run sandboxed (no file system, no network, CPU/memory limits).
- Template rendering uses Go text/template with safe functions only.
- All transform types idempotent.

## Cross-Context Rules

- Deleting a Connection cascades-check: no active Integrations reference it.
- Deleting an Integration cascades to its Endpoints, FieldMappings, Schedules.
- Execution records reference IntegrationID + EndpointID (soft delete only; history retained).
- Correlation ID propagates end-to-end: trigger → connector → executor → store → NATS event.

## Transaction Boundaries

- PostgreSQL: Integration/Connection/Endpoint/FieldMapping/Execution metadata changes are ACID in a single transaction per request.
- MongoDB: response data writes are separate; eventual consistency with PostgreSQL metadata accepted.
- NATS: events published after successful MongoDB write (outbox pattern) — at-least-once.
