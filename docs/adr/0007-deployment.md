# ADR-007: Deployment Strategy

## Status
Proposed

## Context
IntegrationHub must support local development, VPS, Docker Swarm, and Kubernetes deployment paths.

## Decision

| Tier | Approach | Notes |
|------|----------|-------|
| Local | Docker Compose (dev/test/prod profiles) | `make up`, healthchecks, non-root images |
| VPS | Compose + Nginx + SSL | Backup/restore, monitoring included |
| Docker Swarm | Overlay network, secrets, configs, replicas, rolling update | Infrastructure/docker/swarm/ |
| Kubernetes | Deployment, Service, Ingress, ConfigMap, Secret, StatefulSet (Postgres/Mongo), PV, probes, HPA, NetworkPolicy, Helm skeleton | infrastructure/kubernetes/ |
| Cloud | Documentation + Terraform skeleton only | No real resource creation; cost/risk/security/backup comparison |

Each deployment doc: Prerequisites, Environment, Secrets, Networking, Database setup, Backup/Restore, SSL, Reverse proxy, Monitoring, Logging, Scaling, Rollback, DR, Security hardening, Troubleshooting.

## Consequences
- Docker-first is an absolute rule (CLAUDE.md decision 2): no host runtime/toolchain usage.
- Kubernetes/Helm are skeletons only; validated manifests, no live cluster apply.
- Cloud is documented, not provisioned.
