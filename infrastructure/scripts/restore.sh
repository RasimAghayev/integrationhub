#!/usr/bin/env sh
# Restore PostgreSQL and MongoDB into the local compose stack from a backup dir.
# Usage: ./restore.sh <backup-dir>
set -eu

DIR="${1:?usage: restore.sh <backup-dir>}"
PGPASSWORD="${POSTGRES_PASSWORD:-devpassword}"
MONGO_USER="${MONGO_ROOT_USER:-root}"
MONGO_PASS="${MONGO_ROOT_PASSWORD:-devpassword}"

echo "Restoring PostgreSQL from $DIR/postgres.sql.gz"
gunzip -c "$DIR/postgres.sql.gz" | \
  docker exec -i -e PGPASSWORD="$PGPASSWORD" integrationhub-postgres \
  psql -U integrationhub -d integrationhub

echo "Restoring MongoDB from $DIR/mongodb.archive.gz"
gunzip -c "$DIR/mongodb.archive.gz" | \
  docker exec -i integrationhub-mongodb \
  mongorestore --archive --gzip --drop -u "$MONGO_USER" -p "$MONGO_PASS" --authenticationDatabase admin

echo "Restore complete from $DIR"