# API Reference (v1)

Base URL: `http://localhost:8080`
All `/api/v1` routes except `POST /api/v1/auth/token`, `POST /api/v1/auth/refresh`
and the webhook receiver require `Authorization: Bearer <jwt>` or `X-API-Key: <key>`.

## Authentication

### POST /api/v1/auth/token
Exchange an admin API key for a short-lived JWT.

```json
{ "grant_type": "api_key" }
```
Headers: `X-API-Key: <admin key>` → `200 { "access_token": "...", "refresh_token": "...", "expires_in": 3600 }`

### POST /api/v1/auth/refresh
Rotate a refresh token and mint a new access token.

```json
{ "refresh_token": "..." }
```
→ `200 { "access_token": "...", "refresh_token": "...", "expires_in": 3600 }`.
Reusing a rotated token revokes the whole token family (`401`).

## Connections

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/connections` | List connections (paginated) |
| POST | `/api/v1/connections` | Create a connection |
| GET | `/api/v1/connections/{id}` | Get one |
| PUT | `/api/v1/connections/{id}` | Update |
| DELETE | `/api/v1/connections/{id}` | Delete |
| POST | `/api/v1/connections/{id}/test` | Send a probe request |
| GET | `/api/v1/connections/{id}/oauth/authorize` | Start OAuth2 authorization |
| GET | `/api/v1/connections/{id}/oauth/callback` | OAuth2 callback |
| POST | `/api/v1/connections/{id}/oauth/refresh` | Rotate the access token |

Connection payload:

```json
{
  "name": "GitHub",
  "type": "rest",
  "auth_type": "api_key",
  "config": { "base_url": "https://api.github.com", "api_key_header": "Authorization" },
  "credentials": { "api_key": "ghp_..." }
}
```
`credentials` is write-only and never returned.

## Integrations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/integrations` | List |
| POST | `/api/v1/integrations` | Create |
| GET | `/api/v1/integrations/{id}` | Get |
| PUT | `/api/v1/integrations/{id}` | Update |
| DELETE | `/api/v1/integrations/{id}` | Delete |
| POST | `/api/v1/integrations/{id}/execute` | Run an endpoint (sync or async) |
| POST | `/api/v1/integrations/{id}/mapping/preview` | Preview mappings on a sample payload |

Integration payload:

```json
{
  "name": "GitHub Issues",
  "type": "rest",
  "connection_id": "0f0c...",
  "endpoints": [{ "name": "list", "method": "GET", "path": "/repos/o/r/issues" }],
  "mappings": [{ "source": "data.id", "target": "id", "transform": "to_int", "required": true }],
  "transform": { "type": "none" }
}
```
`transform.type` is one of `none`, `template` (with `template`) or `lua` (with
`script`; the script sets a global `out` table).

Execute body: `{ "endpoint": "list", "body": "{}", "async": true }` →
`200` with the execution, or `202` when `async` is true.

## Executions

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/executions` | List (`integration_id`, `limit`, `offset`) |
| GET | `/api/v1/executions/dead` | Dead-letter queue |
| GET | `/api/v1/executions/{id}` | Get one |
| POST | `/api/v1/executions/{id}/replay` | Requeue a dead execution |
| GET | `/api/v1/executions/counts` | Counts by status |

## Schedules

| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/api/v1/schedules` | List / create |
| GET/PUT/DELETE | `/api/v1/schedules/{id}` | Get / update / delete |

```json
{ "name": "nightly", "integration_id": "...", "endpoint": "list", "cron": "0 3 * * *", "enabled": true }
```

## Data API

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/data/schema/{integrationId}` | Inferred response schema |
| GET | `/api/v1/data/export?integration_id=&format=csv\|json&limit=` | Export responses |

## Webhooks (no auth header, HMAC verified)

`POST /api/v1/webhooks/{integrationId}/{endpoint}`

Headers: `webhook-signature: t=<unix>,v1=<hex>` (or `X-Hub-Signature-256`),
optional `webhook-id` for replay suppression. The secret comes from the
connection's `config.webhook_secret`. Returns `202` with the queued execution.

## Operations

| Method | Path | Description |
|--------|------|-------------|
| GET | `/` | Service metadata |
| GET | `/healthz` | Liveness |
| GET | `/readyz` | Readiness (postgres, redis, nats) |
| GET | `/metrics` | Prometheus exposition |

## Errors

```json
{ "error": { "code": "bad_request", "message": "name is required" } }
```
`400` validation, `401` unauthenticated, `403` forbidden, `404` not found,
`413` payload too large, `415` unsupported media type, `422` transform failure,
`429` rate limited (with `Retry-After`), `5xx` server error.