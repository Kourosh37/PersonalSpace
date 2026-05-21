#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 2 ]]; then
  echo "Usage: ./scripts/create-admin.sh <username> <password>"
  exit 1
fi

docker compose run --rm backend /app/create-admin --username "$1" --password "$2"