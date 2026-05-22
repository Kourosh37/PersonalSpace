# Space

Space is a self-hosted private file manager with Docker-first deployment, authenticated file storage, resumable uploads, public sharing, previews, admin controls, backups, and production observability.

The project is packaged as one application image plus PostgreSQL and Redis. It is designed to sit behind your existing reverse proxy; Space does not ship its own Caddy/Nginx/Traefik service.

## Documentation

Start here:

- [Documentation Index](docs/index.md)

Core guides:

- [Product Features](docs/product-features.md)
- [User Guide](docs/user-guide.md)
- [Admin Guide](docs/admin-guide.md)
- [Deployment Guide](docs/deployment-guide.md)
- [Configuration Reference](docs/configuration-reference.md)
- [Operations Guide](docs/operations-guide.md)
- [Testing Guide](docs/testing-guide.md)
- [Troubleshooting](docs/troubleshooting.md)

Technical references:

- [Architecture](docs/architecture.md)
- [API Contract](docs/api-contract.md)
- [Security Operations](docs/security-operations.md)
- [Security Threat Model](docs/security-threat-model.md)
- [Offline Deployment](docs/offline-deployment.md)
- [Reverse Proxy Guide](docs/reverse-proxy.md)
- [Compatibility Matrix](docs/compatibility-matrix.md)
- [Alerting Policy](docs/alerting-policy.md)

## Quick Start

See [Deployment Guide](docs/deployment-guide.md) for the full setup flow.

Minimal local flow:

```bash
cp .env.example .env
docker compose build
./scripts/migrate.sh
./scripts/start.sh
./scripts/create-admin.sh admin strong-password-change-me
```

Then open:

```text
http://localhost:${SPACE_APP_PORT:-3000}
```
