# ADR-002: Data Flow (Pull + Push + Transform)

## Status
Proposed

## Context
IntegrationHub must support bidirectional data flow: pulling data from 3rd party APIs, pushing data to 3rd party APIs, and transforming data in between. Execution modes include cron scheduling, webhook triggers, and on-demand manual execution.

## Decision
Execution flow per trigger:

```
Trigger (Schedule/Webhook/Manual/Event)
  → Load Integration config
  → Build HTTP request (template + mapping)
  → Execute with retry/circuit breaker
  → Parse response
  → Apply field mappings (JSONPath)
  → Apply transformations (visual/template/code)
  → Store in MongoDB
  → Publish event (NATS)
  → Notify (webhook/callback)
```

- Pull mode: scheduler/executor initiates outbound HTTP request to 3rd party API
- Push mode: webhook receiver accepts inbound webhook, stores payload, optionally forwards to 3rd party
- Transform mode: field mappings + transform rules applied between parse and store
- Concurrency: worker pool (goroutines, default 10), per-integration concurrency limit, token-bucket rate limiting per connection
- Retry: exponential backoff with jitter, max 3 retries, retryable on 5xx/timeout/connection errors, non-retryable on 4xx (except 429), DLQ after max retries
- Circuit breaker: per connection (closed → open → half-open)

## Consequences
- Context cancellation and worker pool limits prevent goroutine leaks
- DLQ prevents data loss on repeated failures
- NATS publish provides at-least-once event delivery for downstream consumers
- Idempotent consumers required (replay safety)

## Alternatives Considered
- Synchronous request/response only: rejected because webhook-triggered and scheduled execution need async processing
- In-memory queue: rejected because durability and at-least-once delivery require NATS JetStream
