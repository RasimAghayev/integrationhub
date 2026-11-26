# Operator Manual

## Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `INTEGRATIONHUB_SERVER_PORT` | `8080` | HTTP port |
| `INTEGRATIONHUB_DB_HOST/PORT/USER/PASSWORD/NAME` | compose values | PostgreSQL |
| `INTEGRATIONHUB_REDIS_ADDR/PASSWORD` | compose values | Redis (rate limiting) |
| `INTEGRATIONHUB_NATS_URL` | empty | Event publishing; empty logs events only |
| `INTEGRATIONHUB_SECRET_KEY` | empty | base64 32-byte AES key; empty disables credential encryption |
| `INTEGRATIONHUB_JWT_SECRET` | empty | HS256 signing key; empty disables JWT auth |
| `INTEGRATIONHUB_JWT_TTL` | `1h` | Token lifetime |
| `INTEGRATIONHUB_ADMIN_API_KEY` | empty | Seeded as an admin API key at startup |
| `INTEGRATIONHUB_RBAC_POLICY` / `_FILE` | default policy | Casbin policy CSV |
| `INTEGRATIONHUB_RATE_PER_SEC` / `_BURST` | `100` / `200` | Per-IP rate limit |
| `INTEGRATIONHUB_CORS_ORIGINS` | empty | Comma-separated allowed origins |
| `INTEGRATIONHUB_EXEC_WORKERS` | `4` | Concurrent executions |
| `INTEGRATIONHUB_EXEC_POLL_MS` | `1000` | Worker poll interval |
| `INTEGRATIONHUB_SCHED_TICK_MS` | `60000` | Scheduler tick |
| `INTEGRATIONHUB_OTEL_ENDPOINT` | empty | OTLP/gRPC traces; empty disables tracing |
| `INTEGRATIONHUB_OTEL_SAMPLE_RATIO` | `1.0` | Trace sample ratio |
| `INTEGRATIONHUB_OTEL_INSECURE` | `true` | Plaintext OTLP |

## Start and stop

```bash
make up          # build + start postgres, redis, nats, server
make logs        # follow logs
make down        # stop
```

Migrations are embedded and applied at startup; `schema_migrations` records the
applied version. There is no down-migration: roll back code, not schema.

## Health

- `GET /healthz` — process alive
- `GET /readyz` — postgres, redis and nats reachable
- `GET /metrics` — Prometheus exposition

## Backup and restore

See `docs/runbooks/backup-restore.md`. Verify a restore monthly.

## Scaling notes

- The service is stateless apart from the database; scale horizontally.
- Rate limiting is per process when Redis is not configured.
- The scheduler and execution worker run in-process; run exactly one scheduler
  instance per environment, or shard by database advisory lock (not implemented).
- Traces are batched; a slow collector never blocks requests.

## Common tasks

```bash
# create an admin key (printed once at startup when INTEGRATIONHUB_ADMIN_API_KEY is set)
curl -s -X POST localhost:8080/api/v1/auth/token -H 'X-API-Key: <admin>'

# load test
make loadtest ARGS="-integration <id> -workers 20 -duration 30s"

# replay dead executions
curl -s localhost:8080/api/v1/executions/dead -H 'X-API-Key: <key>'
```

## Troubleshooting

| Area | Check |
|------|-------|
| Startup fails | `INTEGRATIONHUB_SECRET_KEY` must be base64 of 32 bytes |
| 401 on every call | `INTEGRATIONHUB_JWT_SECRET` changed, or the key was deleted |
| Executions dead | Upstream 5xx/timeouts; inspect `error` on the execution |
| No traces | `INTEGRATIONHUB_OTEL_ENDPOINT` unset or collector unreachable |
| Events missing | NATS down; events are logged instead |