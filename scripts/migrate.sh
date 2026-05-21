#!/usr/bin/env bash
set -euo pipefail

docker compose run --rm backend /app/migrate up