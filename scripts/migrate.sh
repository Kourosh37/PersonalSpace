#!/usr/bin/env bash
set -euo pipefail

docker compose run --rm app /app/bin/migrate up
