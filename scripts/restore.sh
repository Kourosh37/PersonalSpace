#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "Usage: ./scripts/restore.sh <backup.sql>"
  exit 1
fi

backup_file="$1"
if [[ ! -f "$backup_file" ]]; then
  echo "Backup file not found: $backup_file"
  exit 1
fi

cat "$backup_file" | docker compose exec -T postgres psql -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}"
echo "Restore completed"