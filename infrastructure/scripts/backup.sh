#!/usr/bin/env sh
# Backup PostgreSQL and MongoDB from the local compose stack.
# Usage: ./backup.sh [output-dir]
set -eu

TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT="${1:-backups/$TS}"
PGPASSWORD="${POSTGRES_PASSWORD:-devpassword}"
MONGO_USER="${MONGO_ROOT_USER:-root}"
MONGO_PASS="${MONGO_ROOT_PASSWORD:-devpassword}"

mkdir -p "$OUT"

echo "Backing up PostgreSQL -> $OUT/postgres.sql.gz"
docker exec -e PGPASSWORD="$PGPASSWORD" integrationhub-postgres \
  pg_dump -U integrationhub -d integrationhub | gzip > "$OUT/postgres.sql.gz"

echo "Backing up MongoDB -> $OUT/mongodb.archive.gz"
docker exec integrationhub-mongodb \
  mongodump --archive --gzip -u "$MONGO_USER" -p "$MONGO_PASS" --authenticationDatabase admin \
  > "$OUT/mongodb.archive.gz"

echo "Backup complete: $OUT"