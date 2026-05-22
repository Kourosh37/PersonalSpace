#!/usr/bin/env bash
set -euo pipefail

mkdir -p backups

docker compose up -d postgres app >/dev/null

app_container_id="$(docker compose ps -q app)"
if [[ -z "$app_container_id" ]]; then
  echo "App container is not running."
  exit 1
fi

storage_volume="$(
  docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data/storage"}}{{.Name}}{{end}}{{end}}' "$app_container_id"
)"
if [[ -z "$storage_volume" ]]; then
  echo "Could not detect storage volume name from app container."
  exit 1
fi

ts="$(date +%Y%m%d-%H%M%S)"
backup_dir="backups/space-backup-${ts}"
mkdir -p "$backup_dir"

db_file="${backup_dir}/db.sql"
storage_file="${backup_dir}/storage.tar.gz"

docker compose exec -T postgres pg_dump -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" > "$db_file"

docker run --rm \
  -v "${storage_volume}:/volume:ro" \
  -v "$(pwd)/${backup_dir}:/backup" \
  alpine:3.20 \
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
