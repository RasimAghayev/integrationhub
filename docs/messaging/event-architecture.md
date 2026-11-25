# Event Architecture

## NATS JetStream Streams & Subjects

| Stream | Subject | Description | Retention |
|--------|---------|-------------|-----------|
| executions | execution.created | Execution started | 7 days |
| executions | execution.completed | Execution succeeded | 7 days |
| executions | execution.failed | Execution failed | 7 days |
| executions | execution.retrying | Execution retrying | 7 days |
| webhooks | webhook.received | Incoming webhook ingested | 7 days |
| integrations | integration.status.changed | Integration status transition | 7 days |
| dlq | dlq.exhausted | Execution DLQ after max retries | 30 days |

## Event Schema

```json
{
  "event_id": "uuid",
  "event_type": "execution.completed",
  "timestamp": "2026-11-25T18:40:00Z",
  "version": 1,
  "correlation_id": "uuid",
  "causation_id": "uuid",
  "source": "integrationhub-server",
  "payload": { ... },
  "metadata": { ... }
}
```

- `event_id`: unique per event (dedup key).
- `correlation_id`: end-to-end trace across trigger→store→NATS.
- `causation_id`: the event that caused this one (command→event link).
- `version`: schema version for evolution.

## Event Versioning
- New fields are additive only; consumers ignore unknown fields.
- Breaking changes increment `version` and run old+new consumers in parallel during migration.

## Transactional Outbox Pattern
1. Application commits business changes + outbox row in a single PostgreSQL transaction.
2. Outbox publisher polls unsent outbox rows and publishes to NATS JetStream.
3. On publish success, marks outbox row as sent.
4. If NATS is down, outbox rows accumulate; publisher resumes when broker recovers (at-least-once).

## Idempotent Consumers
- Consumers dedup by `event_id` (processed-event cache in Redis/PostgreSQL).
- Retry-safe: executing the same event twice produces the same result.
- Dead Letter Queue: after max retries, event moves to DLQ stream for manual inspection.

## Event Replay
- NATS JetStream durable streams support replay by sequence or timestamp.
- Replay emits events with original `event_id`; consumers dedup already-processed IDs.
