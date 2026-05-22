#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 2 ]]; then
  echo "Usage: ./scripts/restore.sh <backup_dir> [--force]"
  exit 1
fi

backup_dir="$1"
force_flag="${2:-}"

if [[ ! -d "$backup_dir" ]]; then
  echo "Backup directory not found: $backup_dir"
  exit 1
fi

db_file="${backup_dir}/db.sql"
storage_file="${backup_dir}/storage.tar.gz"
if [[ ! -f "$db_file" ]]; then
  echo "Missing file: $db_file"
  exit 1
fi
if [[ ! -f "$storage_file" ]]; then
  echo "Missing file: $storage_file"
  exit 1
fi

if [[ "$force_flag" != "--force" ]]; then
  echo "Restore is destructive and will overwrite database and storage volume."
  echo "Run again with --force to continue."
  exit 1
fi

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

cat "$db_file" | docker compose exec -T postgres psql -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}"

docker run --rm \
  -v "${storage_volume}:/volume" \
  -v "$(pwd)/${backup_dir}:/backup:ro" \
  alpine:3.20 \
  sh -c 'find /volume -mindepth 1 -delete && tar xzf /backup/storage.tar.gz -C /volume'

echo "Restore completed from: ${backup_dir}"
