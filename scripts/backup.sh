#!/usr/bin/env bash
set -euo pipefail

source "$(dirname "$0")/lib/compose.sh"

mkdir -p backups

docker compose up -d postgres >/dev/null
wait_for_postgres

storage_volume="$(storage_volume_name)"
ensure_storage_volume "$storage_volume"

ts="$(date +%Y%m%d-%H%M%S)"
backup_dir="backups/space-backup-${ts}"
mkdir -p "$backup_dir"

db_file="${backup_dir}/db.sql"
storage_file="${backup_dir}/storage.tar.gz"

docker compose exec -T postgres pg_dump --clean --if-exists -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" > "$db_file"

docker run --rm \
  -v "${storage_volume}:/volume:ro" \
  -v "$(pwd)/${backup_dir}:/backup" \
  postgres:16-alpine \
  sh -c 'tar czf /backup/storage.tar.gz -C /volume .'

cat > "${backup_dir}/manifest.txt" <<EOF
created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
project=$(basename "$(pwd)")
postgres_db=${POSTGRES_DB:-space}
postgres_user=${POSTGRES_USER:-space}
storage_volume=${storage_volume}
db_file=db.sql
storage_file=storage.tar.gz
EOF

echo "Backup created: ${backup_dir}"
