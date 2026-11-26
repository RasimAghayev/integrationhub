# Production Hardening Checklist

Status legend: `[x]` implemented in this repository, `[ ]` operational task for
the deployment environment.

## 1. Authentication and authorization

- [x] JWT (HS256) and hashed API keys, both accepted on `/api/v1`
- [x] RBAC via Casbin with `admin`, `operator`, `viewer` roles and wildcard resources
- [x] Admin API key bootstrap from `INTEGRATIONHUB_ADMIN_API_KEY`
- [x] Per-resource authorization middleware, default-deny for unknown roles
- [ ] Rotate `INTEGRATIONHUB_JWT_SECRET` and all API keys every 90 days
- [ ] Store secrets in a managed secret store, never in `.env` files on disk

## 2. Input and transport security

- [x] Strict JSON decoding: 1 MiB limit, unknown fields rejected, trailing data rejected
- [x] 415 for wrong content type, 400 for malformed bodies
- [x] Webhook payload cap (1 MiB) and Standard-Webhooks HMAC verification
- [x] Security headers, request IDs, 30s handler timeout, 1 MiB header cap
- [x] Per-IP token-bucket rate limiting with `Retry-After`
- [ ] Terminate TLS at the ingress and redirect plain HTTP
- [ ] Enable HSTS at the ingress once TLS is confirmed

## 3. Secrets and credentials

- [x] AES-256-GCM envelope encryption with versioned ciphertext (v1)
- [x] Credentials are write-only through the API
- [x] OAuth2 state is HMAC-signed and expiring; PKCE S256 supported
- [ ] Restrict `INTEGRATIONHUB_SECRET_KEY` to the service identity
- [ ] Document and rehearse key rotation (re-encrypt all connections)

## 4. Data protection

- [x] Parameterized queries only; no string-built SQL
- [x] Migrations are embedded and idempotent, tracked in `schema_migrations`
- [ ] Enable PostgreSQL TLS (`sslmode=require`) for managed instances
- [ ] Enable encryption at rest on the database volume
- [ ] Schedule encrypted backups and verify a restore monthly

## 5. Availability and resilience

- [x] Retries with exponential backoff and jitter
- [x] Per-connection circuit breaker and rate limiter
- [x] Bounded global worker pool and per-integration concurrency limit
- [x] Dead-letter queue with replay endpoint
- [x] Graceful shutdown with a 10s drain and 5s trace flush
- [ ] Configure readiness gates so traffic only reaches a warm instance
- [ ] Load test with `go run ./cmd/loadtest` before each release

## 6. Observability

- [x] Structured JSON logs with request IDs
- [x] Prometheus metrics: HTTP, executor, breaker, pool stats
- [x] OpenTelemetry traces over OTLP/gRPC, ratio sampled
- [x] `/healthz` and `/readyz` probes
- [ ] Alert on 5xx rate, breaker open, dead-letter growth, queue age
- [ ] Ship traces to a collector with a 30 day retention policy

## 7. Dependency and supply chain

- [x] Pinned Go toolchain (`go 1.26.0`) and `GOTOOLCHAIN=local`
- [x] `gosec` in CI (advisory), `golangci-lint` v2 blocking
- [x] Distroless non-root runtime image
- [ ] Enable Dependabot/Renovate and review weekly
- [ ] Enable branch protection requiring review and green checks

## 8. Sign-off

| Area | Owner | Date | Notes |
|------|-------|------|-------|
| Auth/RBAC | | | |
| Secrets | | | |
| Data | | | |
| Availability | | | |
| Observability | | | |