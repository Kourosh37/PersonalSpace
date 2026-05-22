#!/usr/bin/env bash

compose_project_name() {
  if [[ -n "${COMPOSE_PROJECT_NAME:-}" ]]; then
    printf '%s\n' "$COMPOSE_PROJECT_NAME"
    return
  fi
  basename "$(pwd)" | tr '[:upper:]' '[:lower:]' | tr -c 'a-z0-9' '_'
}

storage_volume_name() {
  local app_container_id
  app_container_id="$(docker compose ps -q app 2>/dev/null || true)"
  if [[ -n "$app_container_id" ]]; then
    local from_container
    from_container="$(
      docker inspect -f '{{range .Mounts}}{{if eq .Destination "/data/storage"}}{{.Name}}{{end}}{{end}}' "$app_container_id"
    )"
    if [[ -n "$from_container" ]]; then
      printf '%s\n' "$from_container"
      return
    fi
  fi

  local project
  project="$(compose_project_name)"
  printf '%s\n' "${project}_space_storage"
}

ensure_storage_volume() {
  local volume_name="$1"
  if ! docker volume inspect "$volume_name" >/dev/null 2>&1; then
    docker volume create "$volume_name" >/dev/null
  fi
}

wait_for_postgres() {
  local attempts="${1:-60}"
  local i
  for ((i = 1; i <= attempts; i++)); do
    if docker compose exec -T postgres pg_isready -U "${POSTGRES_USER:-space}" -d "${POSTGRES_DB:-space}" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  echo "Postgres did not become ready in time." >&2
  return 1
}
