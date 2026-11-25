# Architecture Document

IntegrationHub — Go-based API integration platform.

## 1. Overview

IntegrationHub connects to 3rd party APIs (REST, webhooks, GraphQL), stores responses internally, and provides a web-based admin UI for configuring integrations, field mappings, request body templates, and execution triggers (scheduled + event-driven + on-demand).

## 2. Goals & Non-Goals

**In scope:** REST/GraphQL connections, incoming webhooks, UI-based connection config, field mapping (JSONPath), request body templates, cron/webhook/on-demand triggers, bidirectional pull+push+transform, visual/template/code transforms, PostgreSQL (config) + MongoDB (data), OAuth2/API key/JWT/basic/custom auth, retry/rate limit/circuit breaker/DLQ, audit logging, observability.

**Out of scope:** gRPC/SOAP, multi-tenancy with billing, mobile app.

## 3. Technology Stack

See ADR-001 (docs/adr/0001-technology-stack.md).

## 4. Architecture Style

Modular Monolith + Event-Driven Architecture (EDA), Clean Architecture, DDD, Repository Pattern, Dependency Injection.

Bounded contexts: Integration, Connection, Mapping, Execution, Transform.

## 5. Domain Model

See docs/architecture/domain-model.md.

## 6. C4 Diagrams

- Context: docs/architecture/c4-context.md
- Container: docs/architecture/c4-container.md
- Component: docs/architecture/c4-component.md

## 7. Data Flow

Trigger → Load config → Build request → Execute (retry/circuit breaker) → Parse → Apply mappings → Apply transforms → Store (MongoDB) → Publish event (NATS) → Notify.

See ADR-002 (docs/adr/0002-data-flow.md).

## 8. Persistence

See ADR-004 (docs/adr/0004-persistence.md) and docs/database/responsibility-matrix.md.

## 9. Event Architecture

See docs/messaging/event-architecture.md.

## 10. Security

See ADR-005 (docs/adr/0005-authentication.md) and docs/security/architecture.md.

## 11. Observability

See ADR-006 (docs/adr/0006-observability.md).

## 12. Deployment

See ADR-007 (docs/adr/0007-deployment.md) and docs/deployment/strategy.md.

## 13. ADR Index

| ADR | Title | Status |
|-----|-------|--------|
| 0001 | Technology Stack | Approved |
| 0002 | Data Flow | Approved |
| 0003 | Transformation Engine | Approved |
| 0004 | Persistence Strategy | Proposed |
| 0005 | Authentication & Authorization | Proposed |
| 0006 | Observability | Proposed |
| 0007 | Deployment Strategy | Proposed |
