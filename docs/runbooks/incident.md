# Incident Runbook

## Symptoms and first actions

| Symptom | First check | Action |
|---------|-------------|--------|
| 5xx rate high | `integrationhub_http_requests_total{path=~"/api/.*"}` | Check dependency health, then roll back if the release correlates |
| Executions piling up | `integrationhub_execution_requests_total{outcome="error"}` | Inspect breaker gauge; open breaker means the upstream is failing |
| Dead-letter growing | `GET /api/v1/executions/dead` | Replay with `POST /api/v1/executions/{id}/replay` after fixing the cause |
| `/readyz` failing | `logs` for postgres/redis/nats errors | Restore the failing dependency; the pod is pulled from rotation automatically |
| Trace export errors | collector logs | Disable `INTEGRATIONHUB_OTEL_ENDPOINT` to stop the noise, fix the collector |

## Degraded dependency handling

- PostgreSQL down: the service fails readiness and stops receiving traffic.
  Executions already queued remain in the database and resume after recovery.
- Redis down: rate limiting falls back to per-process buckets; behaviour is
  approximate but the service stays available.
- NATS down: execution events are logged instead of published; nothing is lost
  from the database, but downstream consumers see a gap.
- OTLP collector down: spans are dropped by the batch processor; the request
  path is unaffected.

## Rollback

1. `kubectl rollout undo deployment/integrationhub` (or the platform equivalent).
2. Migrations are additive and idempotent; no down-migration is required for a
   code rollback.
3. Verify `/healthz`, `/readyz` and a sample execution.

## Post-incident

- Record the timeline, affected integrations and the replay set.
- File follow-ups for any `[ ]` item in `docs/security/hardening.md` that failed.