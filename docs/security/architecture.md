# Security Architecture

## Authentication

| Channel | Method | Details |
|---------|--------|---------|
| Admin UI | JWT + refresh rotation | Access token short-lived; refresh token rotated on use |
| API | API key + JWT | API key in `X-API-Key` header; JWT for scoped access |
| Webhook receiver | HMAC-SHA256 | `X-IntegrationHub-Signature` header; secret per integration endpoint |
| 3rd party (outbound) | OAuth2 / API key / Basic / Custom | Per-connection auth config, encrypted at rest |

## Authorization (Casbin RBAC)

Roles: Admin, Editor, Viewer, Operator.

Permissions:
- `integration:read/write/delete`
- `connection:read/write`
- `execution:trigger`
- `data:read/export`

Enforced in the backend (middleware + service layer), never frontend-only.

## Secrets Management

- Credentials encrypted at rest with AES-256-GCM.
- Key rotation supported (versioned encryption keys).
- No secrets in logs, error responses, or trace attributes.
- Environment variable fallback for bootstrap (never committed).
- Docker secrets for database passwords, Redis password, JWT secret.

## Network Security

- TLS 1.3 everywhere (Nginx termination + internal mTLS where applicable).
- Webhook IP allowlisting per integration endpoint.
- Rate limiting per client (token bucket, per connection).
- CORS configured (allowed origins only).
- Security headers: HSTS, X-Content-Type-Options, X-Frame-Options, CSP.

## Audit Logging

- Records: who, what, when, correlation ID, outcome.
- No secrets or personal data in audit entries.
- Append-only in MongoDB; retention 365 days.
