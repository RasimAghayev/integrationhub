# ADR-005: Authentication & Authorization

## Status
Proposed

## Context
IntegrationHub needs authentication for the Admin UI, API, and webhooks, plus authorization (RBAC) and encrypted secret storage.

## Decision

### Authentication
- Admin UI: JWT + refresh token rotation.
- API: API key + JWT.
- Webhook receiver: HMAC-SHA256 signature verification.
- 3rd party connections: OAuth2 (authorization code), API key, basic auth, custom headers.

### Authorization
Casbin RBAC with roles: Admin, Editor, Viewer, Operator.
Permissions: integration:read/write/delete, connection:read/write, execution:trigger, data:read/export.
Authorization enforced in the backend (not only frontend).

### Secrets
Credentials encrypted at rest with AES-256-GCM. Key rotation supported. No secrets in logs or responses. Environment variable fallback for bootstrap.

## Consequences
- JWT rotation adds complexity but limits token theft window.
- HMAC webhook verification prevents spoofing.
- Encrypted auth_config requires decryption before every 3rd party call (performance cost, acceptable).
- Casbin RBAC adds a dependency but centralizes authorization policy.

## Alternatives Considered
- Session-based auth: rejected — JWT is stateless and fits API-first design.
- OPA/Rego: rejected — Casbin is simpler and sufficient for RBAC.
