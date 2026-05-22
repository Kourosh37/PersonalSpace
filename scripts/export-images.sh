#!/usr/bin/env bash
set -euo pipefail

mkdir -p images

docker save \
  postgres:16-alpine \
  redis:7-alpine \
  space-app:latest \
  -o images/space-images.tar

echo "Exported images/space-images.tar"
