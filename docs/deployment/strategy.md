# Deployment Strategy

## Local (Docker Compose)
- `infrastructure/docker/compose/compose.dev.yaml`: PostgreSQL, MongoDB, Redis, NATS, server, nginx.
- `make up` builds, verifies (`make verify`), then starts all services.
- Healthchecks on every service; `make health` reports status.
- Non-root container users; Docker secrets for credentials.

## VPS
- Docker Compose + Nginx + SSL (Let's Encrypt).
- Automated backup + restore test.
- Monitoring stack: Prometheus, Grafana, Loki.
- Rolling restart with zero-downtime health checks.

## Docker Swarm
- Overlay network for service isolation.
- Swarm secrets + configs (no env var secrets).
- Replicas per service; rolling update with update delay + failure action pause.
- `infrastructure/docker/swarm/` skeleton.

## Kubernetes
- Deployment, Service, Ingress per component.
- ConfigMap + Secret for configuration/credentials.
- StatefulSet with PV for PostgreSQL and MongoDB.
- Liveness/readiness/startup probes; HPA for CPU-based scaling.
- NetworkPolicy for inter-service isolation.
- Helm chart skeleton: `infrastructure/kubernetes/`.

## Cloud (Documentation Only)
- AWS / GCP / Azure comparison: cost, risk, security, backup, scaling.
- Terraform skeleton: `infrastructure/terraform/` (no real resources provisioned).
- No cloud resources created or paid for.

## Per-Deployment Checklist
Each deployment document covers: Prerequisites, Environment, Secrets, Networking, Database setup, Backup/Restore, SSL, Reverse proxy, Monitoring, Logging, Scaling, Rollback, Disaster Recovery, Security hardening, Troubleshooting.
