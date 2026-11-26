# Backup & Restore Runbook

Local development stack (compose.dev.yaml). For production, schedule these
against your orchestrator and store artifacts off-host.

## Prerequisites

The stack must be running:

```sh
make up
```

## Backup

```sh
POSTGRES_PASSWORD=devpassword \
MONGO_ROOT_USER=root MONGO_ROOT_PASSWORD=devpassword \
./infrastructure/scripts/backup.sh backups/$(date -u +%Y%m%dT%H%M%SZ)
```

Produces:

- `postgres.sql.gz` — `pg_dump` of the `integrationhub` database
- `mongodb.archive.gz` — `mongodump --archive --gzip`

## Restore

```sh
POSTGRES_PASSWORD=devpassword \
MONGO_ROOT_USER=root MONGO_ROOT_PASSWORD=devpassword \
./infrastructure/scripts/restore.sh backups/<timestamp>
```

Restores PostgreSQL via `psql` and MongoDB via `mongorestore --drop`.

## Verify

```sh
curl -s http://localhost:8080/healthz
# expect {"checks":{"nats":true,"postgres":true,"redis":true},"status":"up",...}
```

## Notes

- Redis is treated as ephemeral cache and is not backed up.
- Credentials come from the environment; never commit real secrets.