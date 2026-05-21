#!/usr/bin/env bash
set -euo pipefail

mkdir -p images

docker save \
  caddy:2.9-alpine \
  postgres:16-alpine \
  redis:7-alpine \
  space-frontend:latest \
  space-backend:latest \
  -o images/space-images.tar

echo "Exported images/space-images.tar"
