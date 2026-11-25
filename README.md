# IntegrationHub

Go-based API integration platform connecting to 3rd party APIs (REST, webhooks, GraphQL) with a web-based admin UI for configuring integrations, field mappings, request body templates, and execution triggers (scheduled + event-driven + on-demand).

## Technology Stack

| Layer | Choice |
|-------|--------|
| Language | Go 1.22+ |
| HTTP Router | Chi (chi/v5) |
| OpenAPI | swaggo |
| Database (Config) | PostgreSQL 16 (pgx/v5) |
| Database (Data) | MongoDB 7 (mongo-driver) |
| Cache | Redis 7 (go-redis/v9) |
| Message Queue | NATS JetStream (nats.go) |
| Scheduler | robfig/cron/v3 |
| Auth | golang-jwt/v5 + oauth2 + casbin |
| Transform | gval + text/template + gopher-lua |
| Observability | OpenTelemetry + Prometheus + Loki |
| UI | React + TypeScript + Tailwind + shadcn/ui |
| Containers | Docker, Docker Compose |
| CI/CD | GitHub Actions, Dependabot |

## Architecture

Modular monolith + Event-Driven Architecture (EDA), Clean Architecture, DDD, Repository Pattern, Dependency Injection.

Bounded contexts: Integration, Connection, Mapping, Execution, Transform.

## Getting Started

### Prerequisites

- Docker and Docker Compose only. No host language runtimes.

### Local Development

```bash
make up
```

Builds and starts all services via Docker Compose.

### Testing

```bash
make test
```

Runs the test suite inside Docker containers.

### Verification

```bash
make verify
```

Runs build + test stages. `make up` depends on `make verify`.

## Documentation

Full technical documentation is in `docs/`:
- `docs/architecture/` — C4 diagrams, ADRs, domain boundaries
- `docs/database/` — ERD, responsibility matrix
- `docs/infrastructure/` — Docker, deployment, monitoring
- `docs/api/` — OpenAPI specs
- `docs/security/` — Security architecture
- `docs/testing/` — Test strategy and reports

See `docs/DEPLOYMENT.md` and `docs/OPERATIONS.md` for deployment and operations procedures.

## Community

- [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md) — Contributor Covenant 2.1
- [CONTRIBUTING.md](CONTRIBUTING.md) — How to contribute
- [SECURITY.md](SECURITY.md) — Vulnerability reporting
- [SUPPORT.md](SUPPORT.md) — Support channels
- [CHANGELOG.md](CHANGELOG.md) — Release history
- [ACCESSIBILITY.md](ACCESSIBILITY.md) — Accessibility commitment

## License

[MIT](LICENSE)
