#!/usr/bin/env bash
set -euo pipefail

mkdir -p backups

ts=$(date +%Y%m%d-%H%M%S)
out="backups/space-db-${ts}.sql"

docker compose exec -T postgres pg_dump -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" > "$out"
echo "Database backup created: $out"