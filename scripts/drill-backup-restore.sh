#!/usr/bin/env bash
set -euo pipefail

if [[ ! -f ".env" ]]; then
  echo ".env is required for the backup/restore drill."
  exit 1
fi

ts="$(date +%Y%m%d-%H%M%S)"
export COMPOSE_PROJECT_NAME="${COMPOSE_PROJECT_NAME:-space_drill_${ts}}"
export SPACE_APP_PORT="${DRILL_SPACE_APP_PORT:-0}"

marker_id="drill-${ts}"
marker_body="space-drill-${ts}"
report_file=""

cleanup() {
  if [[ "${DRILL_KEEP_STACK:-0}" != "1" ]]; then
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

storage_volume_name() {
  local app_container_id
  app_container_id="$(docker compose ps -q app)"
  if [[ -z "$app_container_id" ]]; then
    echo "App container is not running." >&2
    return 1
  fi

  docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data/storage"}}{{.Name}}{{end}}{{end}}' "$app_container_id"
}

echo "Starting isolated drill stack: ${COMPOSE_PROJECT_NAME}"
docker compose up -d postgres app >/dev/null
docker compose run --rm app /app/bin/migrate up >/dev/null

storage_volume="$(storage_volume_name)"
if [[ -z "$storage_volume" ]]; then
  echo "Could not detect drill storage volume."
  exit 1
fi

echo "Creating drill marker data."
docker compose exec -T postgres psql -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" >/dev/null <<SQL
CREATE TABLE IF NOT EXISTS disaster_recovery_drill (
  id text primary key,
  marker text not null,
  created_at timestamptz not null default now()
);
DELETE FROM disaster_recovery_drill;
INSERT INTO disaster_recovery_drill (id, marker) VALUES ('${marker_id}', '${marker_body}');
SQL

docker run --rm \
  -v "${storage_volume}:/volume" \
  alpine:3.20 \
  sh -c "mkdir -p /volume/drill && printf '%s' '${marker_body}' > /volume/drill/marker.txt"

echo "Running backup."
backup_output="$(./scripts/backup.sh)"
echo "$backup_output"
backup_dir="$(printf '%s\n' "$backup_output" | awk -F': ' '/Backup created:/ {print $2}' | tail -n1)"
if [[ -z "$backup_dir" || ! -d "$backup_dir" ]]; then
  echo "Backup directory could not be detected."
  exit 1
fi
report_file="${backup_dir}/drill-report.txt"

echo "Mutating data before restore."
docker compose exec -T postgres psql -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" >/dev/null <<SQL
DELETE FROM disaster_recovery_drill;
SQL
docker run --rm \
  -v "${storage_volume}:/volume" \
  alpine:3.20 \
  sh -c "rm -f /volume/drill/marker.txt"

echo "Running restore."
./scripts/restore.sh "$backup_dir" --force >/dev/null

echo "Verifying restored database marker."
restored_db_marker="$(
  docker compose exec -T postgres psql -U "${POSTGRES_USER:-space}" "${POSTGRES_DB:-space}" -At \
    -c "SELECT marker FROM disaster_recovery_drill WHERE id='${marker_id}'"
)"
if [[ "$restored_db_marker" != "$marker_body" ]]; then
  echo "Database marker verification failed."
  exit 1
fi

echo "Verifying restored storage marker."
restored_storage_marker="$(
  docker run --rm \
    -v "${storage_volume}:/volume:ro" \
    alpine:3.20 \
    sh -c "cat /volume/drill/marker.txt"
)"
if [[ "$restored_storage_marker" != "$marker_body" ]]; then
  echo "Storage marker verification failed."
  exit 1
fi

cat > "$report_file" <<EOF
created_at_utc=$(date -u +%Y-%m-%dT%H:%M:%SZ)
compose_project=${COMPOSE_PROJECT_NAME}
backup_dir=${backup_dir}
database_marker=verified
storage_marker=verified
result=passed
EOF

echo "Backup/restore drill passed. Report: ${report_file}"
