# 0009. Web UI as a Separate Vite/React Application

- Status: Accepted
- Date: 2026-11-26

## Context

The admin panel needs forms, tables and a mapping preview. Server-side templates
would couple presentation to the Go binary and make the UI hard to iterate on.
The plan calls for React.

## Decision

The UI lives in `web/` as a Vite + React 19 + TypeScript single-page app. It
talks to the existing JSON API only; there is no GraphQL layer and no
server-side rendering.

- `web/src/api.ts` is the only place that knows about HTTP, auth headers and
  response shapes.
- The dev server proxies `/api` to `http://localhost:8080` (see
  `vite.config.ts`), so no CORS configuration is needed in development.
- The token obtained from `POST /api/v1/auth/token` is kept in `localStorage`.
- Production builds are static assets (`npm run build`) served by any static
  host or the ingress; the Go binary does not serve them.

Pages: dashboard (counters), connections, integrations (with run), mapper
(payload → preview), executions (with dead-letter replay), schedules.

## Consequences

- Two toolchains in CI: `go test ./...` and `npm run build` in `web/`.
- The UI is a client of the public API, so API changes surface as type errors at
  build time rather than silently at runtime.
- Authentication tokens in `localStorage` are readable by any script on the
  origin; the UI must therefore be served from a trusted origin only, and XSS
  defences (React escaping, no `dangerouslySetInnerHTML`) are mandatory.
- No SSR means the dashboard is empty until the first API call resolves; the
  loading path is a spinner-free render of zeroed counters.