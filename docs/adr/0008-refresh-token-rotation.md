# 0008. Rotating Refresh Tokens

- Status: Accepted
- Date: 2026-11-26

## Context

Access tokens are short-lived HS256 JWTs. Long-lived credentials are needed so a
user session survives an access-token expiry without re-presenting the admin API
key. Storing raw refresh tokens in the database would let a database leak be
replayed directly.

## Decision

Refresh tokens are opaque 256-bit random strings. Only their SHA-256 hash is
stored (`refresh_tokens.token_hash`, unique). Each token belongs to a `family`;
rotation issues the replacement in the same family and links the old row to it
via `replaced_by`.

`POST /api/v1/auth/token` returns `access_token` and `refresh_token`.
`POST /api/v1/auth/refresh` validates the presented token, revokes it, and
returns a new access/refresh pair. The refresh record stores the subject and
roles so refresh needs a single lookup.

Rotating a token twice is treated as theft: the second presentation revokes the
whole family (`revoked_at` on every member), forcing re-authentication.

Refresh tokens live 30 days by default (`INTEGRATIONHUB_REFRESH_TTL`).

## Consequences

- A stolen token is usable once; the legitimate client's next refresh invalidates
  the attacker, and vice versa — the family is burned either way.
- The database grows by one row per refresh; expired rows can be pruned with
  `RefreshTokenStore.DeleteExpired`.
- API keys remain the bootstrap credential and are stored hashed as before.
- No cryptography beyond SHA-256 is introduced; tokens are never encrypted
  because they are already high-entropy secrets.