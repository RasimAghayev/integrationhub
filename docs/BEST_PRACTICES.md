# IntegrationHub — 2026 Best Practices (per module / library)

> Standing rule (CLAUDE.md decision 19): before every task and every new feature,
> research the named product/tech for current best practices, refactor existing code
> to those principles, and cite sources. This file is the consolidated research
> baseline. Sources are listed per section; "Status" notes how the current
> IntegrationHub code relates.

---

## 1. Go language, modules, toolchain
- Pin `go` in `go.mod` and use `GOTOOLCHAIN=local` in CI for reproducibility.
- `go vet ./...` + `go test ./...` + `go test -race` as the local/CI baseline.
- Since Go 1.22 the loop variable is per-iteration: **drop `tc := tc`** in table tests.
- Prefer `log/slog` (stdlib) over third-party loggers for new code.
- Status: Go 1.22 pinned; tests run in `golang:1.22` container.

## 2. chi router (`go-chi/chi/v5`)
- Base middleware order: `RequestID` → (one `ClientIPFrom*`) → `Logger` → `Recoverer` → `Timeout`. Logger must come **before** Recoverer.
- `middleware.RealIP` is **deprecated / spoofable** (GHSA-3fxj-6jh8-hvhx). Use exactly one of:
  - `ClientIPFromRemoteAddr` (direct internet), `ClientIPFromHeader("CF-Connecting-IP")`
    (single trusted proxy), `ClientIPFromXFF(cidrs...)` (known proxy CIDRs),
    `ClientIPFromXFFTrustedProxies(n)` (dynamic proxy fleet). Read via `GetClientIP`/`GetClientIPAddr`.
- Use `r.Route`/`r.Group`/`r.With` for sub-routers and per-group middleware; regex params `{id:^[0-9]+$}`.
- Since Go 1.22 `http.ServeMux` supports methods/wildcards; keep chi for per-group middleware, sub-routers, regex.
- Status: chi upgraded to **v5.2.1**; middleware order `RequestID → slogRequestLogger → Recoverer → metrics → Timeout(30s)`. `ClientIPFromRemoteAddr` needs chi ≥ v5.3 (Go 1.24) — until then the client `remote_addr` is logged directly; `RealIP` stays unused.

## 3. PostgreSQL via pgx v5 / pgxpool
- **Always set `MaxConns` yourself** (default `max(4, NumCPU)` is a container trap: 0.5 CPU limit still sees all host cores).
- Sizing: `max_conns ≈ (2 × CPU cores) + 4` per instance; budget from DB `max_connections` ÷ instances; use PgBouncer instead of a bigger pool.
- Set `MaxConnLifetime` (default 1h) below the shortest idle timeout on the path; `MaxConnIdleTime` (30m) to shed idle; `MinConns` small + warm-up query at startup.
- Always pass a **context with deadline** to every pool call (`Acquire` waits unbounded otherwise).
- Export pool stats as metrics: `AcquiredConns, IdleConns, TotalConns, EmptyAcquireCount, AcquireDuration, CanceledAcquireCount`; alert on `AcquireDuration` per request.
- Set `statement_timeout` + `idle_in_transaction_session_timeout` on the role as a backstop.
- Behind PgBouncer transaction mode use `QueryExecModeSimpleProtocol`/`Exec`.
- Status: `newPool` = ParseConfig + MaxConns/MinConns/lifetimes; **pool stats exported** (`integrationhub_pgxpool_stats`); DSN sets `statement_timeout=5000 idle_in_transaction_session_timeout=10000`.

## 4. NATS / JetStream (`nats.go/jetstream`)
- Use the **`jetstream` package** (not legacy `nc.JetStream()`); **pull consumers** are the modern default.
- Core NATS for fire-and-forget/RPC/queue-groups; JetStream only when you need persistence/replay/dedup.
- Every stream MUST have explicit `MaxBytes`/`MaxAge` (unbounded = disk-fill outage). `FileStorage` for durable, `Replicas: 3` for prod.
- Retention: `LimitsPolicy` (event log), `WorkQueuePolicy` (job queue, non-overlapping filters), `InterestPolicy` (auto-cleanup).
- Publishers set `Nats-Msg-Id` (dedup window) and **always check the publish ack** (`js.Publish` sync or bounded async).
- Consumers: durable, `AckExplicit`, ack **after** commit; `MaxAckPending` = backpressure; `MaxDeliver` + `BackOff` bounds poison; `Term` poison, `Nak` transient.
- No native DLQ — build one via `$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES.>`; monitor `num_pending`/`num_ack_pending`; `Drain` on shutdown.
- Status: not yet implemented (Phase 10). Apply these when adding webhooks/events.

## 5. Redis (`redis/go-redis/v9`)
- Configure `PoolSize`, `MinIdleConns`, `MaxActiveConns`, `PoolTimeout` explicitly (default `10×GOMAXPROCS`).
- Timeouts: `DialTimeout`, `ReadTimeout`, `WriteTimeout`; retries `MaxRetries` + `Min/MaxRetryBackoff`.
- v9.23+ has a separate **pipeline pool** (`PipelinePoolSize`, default 10); watch `PipelineStats`.
- Cache-aside: TTL matched to staleness tolerance; **invalidate on write** (delete key), never dual-write; single-flight Lua lock against stampede; namespace keys (`cache:<app>:...`).
- Client-side caching (v9.22+, RESP3, DB 0) is experimental; `MaxStaleness` as safety net.
- Status: not yet wired (planned cache layer).

## 6. MongoDB Go driver (planned)
- **One `Client` per process**, reused (goroutine-safe); never per-request.
- Pool: `maxPoolSize` (default 100), `minPoolSize`, `maxConnecting` (2), `maxIdleTimeMS`, `waitQueueTimeoutMS` (bound waits).
- Index every field used in queries/sorts; prefer **covered queries**; compound index order must match sort; unique indexes for invariants; TTL for expiry.
- Status: planned (execution/audit storage).

## 7. GraphQL via gqlgen (planned)
- **Schema-first** (`.graphqls`), generate code; keep resolvers thin — delegate to a processor/service that is unit-testable.
- gqlgen runs field resolvers **concurrently**: no shared mutable state, no package globals, run `go test -race`.
- Split **heavy** fields into their own resolver (`resolver: true`) so they only run when requested.
- Never smuggle required values through `context`; always propagate `ctx`; inject `time`/`rand`/I/O for determinism.
- Status: planned (Phase 13 Data API).

## 8. Prometheus (`prometheus/client_golang`)
- Instrument in the same file the metric is used; `promauto.With(reg)`; expose `/metrics` via `promhttp.HandlerFor(reg, ...)`.
- Naming: `namespace_subsystem_name`; **never procedurally generate names — use labels**.
- Keep label cardinality **< 10** (avoid `user_id`, `request_id`, `email`, `session_id`); add a CI grep gate.
- Counter (monotonic) vs Gauge (can go down); Summary/Histogram for latency; export timestamps not "time since".
- Status: HTTP counter + `/metrics` in place. **Gap: cardinality CI check; pool/DB metrics.**

## 9. Structured logging (`log/slog`)
- Stable messages + fields; add an `event` field (`payment.failed`); include trace/request ID.
- Never log secrets/tokens/cookies/full bodies; `Error` only for real failures.
- Bind stable attrs with `Logger.With`; use `LogAttrs` for hot paths; `LogValuer` to redact secrets.
- Context-aware methods (`InfoContext`) let handlers extract trace IDs; don't expect auto-extraction.
- Status: slog JSON in place.

## 10. Secrets at rest — AES-256-GCM
- AES-GCM (authenticated) over CBC; **fresh CSPRNG nonce per call, never reuse**; prepend nonce to ciphertext.
- Store key in a secrets manager / env, **never next to ciphertext**; version ciphertexts (`version||nonce||ct||tag`) for rotation.
- Rotate before ~2^32 encryptions (NIST 800-38D); keep old keys for dual-key reads during rotation.
- Add a round-trip + bit-flip (tamper) test.
- Status: AES-256-GCM with round-trip/tamper tests; ciphertexts are **versioned** (`version||nonce||ct||tag`, v1) to allow key rotation.

## 11. OAuth2 / OIDC security (RFC 9700 BCP)
- **PKCE S256 is mandatory** for all authorization-code flows (public *and* confidential); `S256` only (verifier not exposed).
- `state` (or PKCE/nonce) for CSRF; validate on return; never put tokens in URLs/logs.
- Exact redirect-URI matching; prevent open redirectors; `iss` (RFC 9207) against mix-up attacks.
- Short-lived access tokens; **refresh token rotation** for public clients; sender-constrain (mTLS/DPoP) where possible.
- PKCE downgrade: server MUST reject `code_verifier` when no `code_challenge` was present.
- Status: PKCE S256 + HMAC state + rotation implemented. **Gap: `iss` mix-up check, DPoP/mTLS (optional).**

## 12. Templates (`text/template` vs `html/template`)
- **Template text is code** (SSTI, CWE-1336). Never `Parse` user input; keep templates static in the repo.
- Use `html/template` (contextual auto-escaping) for any HTML; never wrap untrusted data in `template.HTML/JS/URL`.
- Pass untrusted input only as **data**; keep render context minimal (no secrets/service clients).
- `text/template` is acceptable for non-HTML (JSON) with `missingkey=error`, parse once, execute to a buffer.
- Status: `template.Render` uses text/template for JSON bodies with `missingkey=error` + buffered (correct). **Rule: if HTML output is ever added, switch to `html/template`.**

## 13. Testing — testcontainers-go
- Prefer **pre-configured modules** (`postgres.Run`, `redis.Run`) over generic containers.
- Register cleanup **before** error check: `testcontainers.CleanupContainer(t, ctr)`.
- Wait strategies: `wait.ForListeningPort` / `ForSQL` / `ForLog(...).WithOccurrence(2)` + `ForAll`; **never `time.Sleep`**; always set startup timeout.
- One container per package via `TestMain` (no `defer` before `os.Exit`); transaction-per-case for isolation; pin image tags; pre-pull in CI.
- Status: store integration tests use testcontainers with `ForLog(...).WithOccurrence(2)`. **Gap: consider `postgres` module + `ForListeningPort`.**

## 14. Table-driven tests
- `t.Run(name, ...)` per case so failures name the row; behavior-focused names.
- `t.Parallel()` first line for independent cases; **no `tc := tc` needed on Go 1.22+**.
- Use maps for order-independence; skip the table for single cases / order-dependent scenarios.
- Status: tests follow this pattern.

## 15. golangci-lint
- Use **v2 config** (`version: "2"`, `linters.default: standard`) with an explicit `enable` list; **pin the release** in CI.
- Run the same command locally and in CI; `--fast-only` for quick local loops.
- v2: per-linter settings under `linters.settings`; paths relative to config file.
- Status: `.golangci.yml` migrated to **v2** (`version: "2"`, `linters.default: standard`, `formatters: gofmt/goimports`); CI runs `golangci/golangci-lint-action@v9` pinned to `version: v2.13`; code is lint-clean (0 issues).

## 16. Docker (multi-stage, distroless)
- Multi-stage: `golang:1.x` build → `distroless/static-debian12:nonroot` (or `scratch` + CA certs).
- `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`; copy `go.mod/go.sum` before source for layer cache; BuildKit `--mount=type=cache`.
- `.dockerignore` excludes `.git`, local env; `ENTRYPOINT` exec form; non-root; pin base **by digest**; rebuild+re-scan on schedule.
- Status: `cmd/server/Dockerfile` is multi-stage `golang:1.22` → `distroless/static-debian12:nonroot`, `CGO_ENABLED=0 -trimpath -ldflags="-s -w"`, BuildKit module/build caches, non-root.

## 17. CI (GitHub Actions)
- Pin action/tool versions; same commands as local; run `go vet`/`test`/`-race`, lint, and image scan (Trivy) as gates.
- Integration job uses Docker (testcontainers); pre-pull images; fail on fixable HIGH/CRITICAL.
- Status: `ci.yml` has build + integration jobs; lint job runs `go vet`, `gofmt`, and `golangci-lint`.

## 18. HTTP server hardening (`net/http`)
- Always construct an explicit `http.Server` with `ReadHeaderTimeout` (Slowloris), `ReadTimeout`, `WriteTimeout`, `IdleTimeout`, `MaxHeaderBytes`.
- Bound request bodies at the handler edge with `http.MaxBytesReader(w, r.Body, n)` → `*http.MaxBytesError` → 413.
- Graceful shutdown: `signal.NotifyContext` → `srv.Shutdown(ctx)` (10s), and **wait for `ListenAndServe` to return** `http.ErrServerClosed` (expected, not fatal).
- Sources: pkg.go.dev/net/http; go.ehabterra.com/security/hardening-services; dixon.cx/blog/tiny-go-http-server-limits-deadlines-shutdown.
- Status: `http.Server` sets `ReadHeaderTimeout/ReadTimeout/WriteTimeout/IdleTimeout/MaxHeaderBytes`; graceful shutdown drains and treats `ErrServerClosed` as normal.

## 19. JSON request decoding
- Enforce `Content-Type: application/json` when present (415 otherwise; missing tolerated for curl); `mime.ParseMediaType` handles `; charset`.
- `dec.DisallowUnknownFields()` so `{"nmae":...}` fails loudly; reject a **second JSON value** (decode into `struct{}{}` must be `io.EOF`).
- Map error types: `*json.SyntaxError` (offset), `*json.UnmarshalTypeError` (field), `io.ErrUnexpectedEOF`, `io.EOF` (empty), `*http.MaxBytesError` (413).
- Sources: gohacks.dev/library/go-decode-json-request-body; alexedwards.net/blog/how-to-properly-parse-a-json-request-body; nalar.dev/reject-unknown-json-fields.
- Status: shared `api.decodeJSON` enforces all of the above; used by connection + integration Create/Update.

## 20. Validation & error mapping
- Layers: limit size → decode → reject unknown fields → validate (required/ranges/relationships) → execute.
- Map domain errors to status: `ErrNotFound`→404, validation→400, body too large→413, bad content-type→415, else 500; never leak internals.
- Keep `domain.Validate()` as the single source of truth for field rules.
- Sources: alexedwards.net; nalar.dev; pkg.go.dev/encoding/json.
- Status: handlers call `domain.Validate()`; `writeNotFound`/`writeError` map errors.

## 21. Distributed tracing (OpenTelemetry)
- Use the OTel **SDK** in the app (API only in libraries); `sdktrace.NewTracerProvider` with `WithBatcher` + resource `service.name`; `defer tp.Shutdown(ctx)`.
- Propagate via W3C `propagation.TraceContext{}`; sampler `ParentBased(TraceIDRatioBased)` in prod, `AlwaysSample` in dev.
- Instrument edges with `otelhttp` (inbound/outbound); **every DB/HTTP call must take `ctx`** or spans won't link; set span status `Error` + `RecordError`.
- Sources: opentelemetry.io/docs/languages/go/instrumentation; .../sampling; encore.dev/articles/opentelemetry-go-guide.
- Status: planned (Phase 4/15). `otelhttp` already an indirect dep via testcontainers.

## 22. Webhook signature verification (Phase 10)
- Sign `timestamp + "." + raw body` with HMAC-SHA256; verify the **raw bytes before parsing**; compare with `hmac.Equal` (constant time), never `==`.
- Accept multiple `v1` signatures and multiple secrets (rotation); reject timestamp outside ±5min tolerance (both directions); cache event IDs ≥ 2× tolerance to block replays.
- One secret per registered webhook; store in a secrets manager; redact signatures from logs; return 401 (no reason) on failure.
- Sources: cheatsheetseries.owasp.org/cheatsheets/Webhook_Security_Cheat_Sheet.html; gohacks.dev/library/go-webhook-receiver.
- Status: planned (Phase 10).

## 23. Configuration & secrets
- Config via env with sane defaults; fail fast on invalid critical config (e.g., bad `INTEGRATIONHUB_SECRET_KEY`).
- Never log secrets/tokens/signatures; keep keys out of images and source; version ciphertexts for rotation.
- Sources: OWASP Secrets Management Cheat Sheet; cheatsheetseries.owasp.org.
- Status: env-driven config; secret key validated at startup; credentials write-only.

## 24. IDs, time & JSON output
- IDs: UUID/string, generated server-side, never client-supplied.
- Time: store UTC, serialize RFC 3339; `omitempty` for optional fields; never serialize `secret_ref`/credentials.
- Sources: pkg.go.dev/encoding/json; pkg.go.dev/time.
- Status: server-generated IDs; `secret_ref` has `json:"-"`; timestamps from DB.

## 25. Pagination & list endpoints
- List endpoints should be bounded: `limit` (default + max) and `offset` or cursor; return a stable order; never unbounded full-table scans.
- Sources: REST API design best practices (Microsoft/Google API design guides).
- Status: **gap** — `List` currently returns all rows; add limit/offset before Phase 13.

## 26. Idempotency for POST
- Support `Idempotency-Key` for create operations (store key + response, replay on retry) to make client retries safe.
- Sources: IETF Idempotency-Key draft; Stripe idempotent requests.
- Status: planned (Phase 15 hardening).

## 27. Concurrency & context
- Stores/handlers must be goroutine-safe (no shared mutable state); pass `r.Context()` to all I/O; run `go test -race`.
- Never start goroutines that outlive the request without a shutdown path.
- Sources: pkg.go.dev/net/http; pkg.go.dev/context.
- Status: handlers pass `r.Context()`; chi `Timeout(30s)` bounds work.

## 28. Rate limiting & CORS
- Add `chi/middleware` rate limiting per client (trusted proxy) and a restrictive CORS policy for the Web UI origin.
- Sources: github.com/go-chi/chi docs; OWASP.
- Status: planned (Phase 15).

## 29. Project layout & dependency injection
- `cmd/` wiring only; `internal/{domain,store,api,oauth,secret,template}`; handlers depend on **interfaces** (`ConnectionStore`, `IntegrationStore`) for testability.
- Keep `domain` free of framework imports; inject dependencies via constructors.
- Sources: golang-standards/project-layout; pkg.go.dev.
- Status: layout + interface-based handlers in place; fakes in tests.

## 30. Execution engine (retry / breaker / rate limit / pool)
- Retry: exponential backoff with **full jitter** (`rand.Int64N`), cap delay, 3–5 attempts; retry 5xx/429/timeouts/conn errors, never 4xx (except 429); honor `Retry-After`; only retry idempotent calls.
- Circuit breaker per connection (closed→open→half-open), one probe at a time; count 5xx/timeouts as failures, not 4xx; `ErrOpen` is a fast, permanent failure for the retry loop (breaker inside the retry loop).
- Rate limit per connection (token bucket, `golang.org/x/time/rate`); bound concurrency with a global worker pool + per-integration semaphore.
- Sources: mubashirtaj.dev/blog/retry-with-exponential-backoff-in-go; gohacks.dev/library/go-circuit-breaker; pkg.go.dev/golang.org/x/time/rate; gohacks.dev/library/go-api-rate-limiting.
- Status: `internal/executor` implements all of the above (breaker/limiter/pool/executor) with unit tests; manual trigger `POST /api/v1/integrations/{id}/execute`.

---

### Cross-cutting checklist (apply to every new feature)
1. Research the named tech, cite sources in the commit/PR.
2. Refactor existing code to the principle before adding new code.
3. Security: no secrets in logs/URLs; validate input; least privilege.
4. Observability: metrics (low cardinality) + structured logs with trace/request ID.
5. Tests: table-driven + integration (testcontainers) for DB/queue/cache paths.
6. Verify: `gofmt`, `go vet`, `go test`, `go vet -tags integration` in `golang:1.22`.