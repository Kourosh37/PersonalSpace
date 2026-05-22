# Space Documentation

Space is a self-hosted private file manager for teams and individuals who need authenticated file storage, resumable uploads, public sharing, preview generation, administrative controls, and Docker-first operations.

## Documentation Map

- `docs/product-features.md`: complete feature list and behavior notes.
- `docs/user-guide.md`: end-user workflows for files, folders, uploads, downloads, previews, and shares.
- `docs/admin-guide.md`: administrator workflows for users, settings, storage, audit logs, and system health.
- `docs/deployment-guide.md`: production deployment model, Docker Compose usage, reverse proxy integration, and upgrade flow.
- `docs/configuration-reference.md`: environment variables and runtime settings.
- `docs/operations-guide.md`: backups, restore, disaster-recovery drills, maintenance, logs, metrics, alerts, and incident response.
- `docs/api-contract.md`: high-level API contract.
- `docs/architecture.md`: service architecture and internal module overview.
- `docs/security-operations.md`: security operations checklist.
- `docs/security-threat-model.md`: threat model and mitigations.
- `docs/testing-guide.md`: local, CI, frontend, backend, and integration testing.
- `docs/troubleshooting.md`: common operational problems and fixes.
- `docs/offline-deployment.md`: build/export/import flow for restricted networks.
- `docs/reverse-proxy.md`: external Caddy/Nginx/Traefik routing examples.
- `docs/compatibility-matrix.md`: validated platform and browser compatibility.
- `docs/alerting-policy.md`: minimum production alerting baseline.

## Production Readiness Summary

Space is packaged as one application image plus PostgreSQL and Redis images. The application image runs the Next.js frontend and Go backend in the `app` service, while the same image is reused by `preview-worker` for asynchronous preview jobs.

The deployment intentionally does not include an internal reverse proxy. Put Space behind an existing external reverse proxy and route traffic to `app:3000`.

Core production capabilities include:

- Argon2id password hashing and server-backed session cookies.
- CSRF origin validation for state-changing API requests.
- Redis-backed rate limiting for sensitive routes.
- Structured JSON logs.
- Prometheus-compatible `/metrics`.
- `/healthz` health endpoint.
- PostgreSQL migrations and startup migration guard.
- File storage on a Docker volume.
- Backup, restore, and disaster-recovery drill scripts.
- Playwright frontend smoke coverage and Go backend unit/integration coverage.
