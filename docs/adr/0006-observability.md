# ADR-006: Observability

## Status
Proposed

## Context
IntegrationHub must be observable: logs, metrics, traces, alerts, health checks.

## Decision
| Signal | Tool | Implementation |
|--------|------|----------------|
| Logs | Loki + slog | Structured JSON, correlation ID |
| Metrics | Prometheus + client_golang | RED metrics per integration |
| Traces | Jaeger + OpenTelemetry | Distributed tracing |
| Alerts | Alertmanager | Error rate, latency, DLQ depth |
| Health | /healthz, /readyz | DB, Redis, NATS, 3rd party connectivity |

## Consequences
- OpenTelemetry adds propagation overhead but gives vendor-neutral tracing.
- Correlation ID flows end-to-end across trigger, connector, executor, store, NATS.
- Health endpoints enable container orchestration probes and load-balancer checks.

## Alternatives Considered
- ELK stack: rejected — Loki + Prometheus + Grafana is simpler for cloud-native/GitOps.
- Zipkin: rejected — Jaeger has better OpenTelemetry integration.
