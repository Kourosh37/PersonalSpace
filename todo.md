# TODO - Space Production Completion

This file tracks all remaining work needed to make the project fully production-ready based on the current codebase state.

## 1) Product feature gaps

- [ ] Complete dashboard file manager parity.
Details: Ensure production-grade behavior for breadcrumb navigation, grid/list switch, details panel, and polish for large directories.

- [ ] Complete admin settings coverage beyond upload/sharing/preview.
Details: Add complete `general`, `download`, and `security` sections with backend validation + audit logging.

- [ ] Fully enforce per-user storage quota.
Details: Enforce quota consistently across multipart, tus, and custom resumable uploads.

- [ ] Complete advanced upload settings.
Details: Support and enforce max concurrent uploads, upload session expiration policy, and optional blocklists (default allow-all).

- [ ] Improve public share UX parity.
Details: Better folder breadcrumb/navigation, clearer state/error UX, and full preview behavior parity with private dashboard.

- [ ] Complete remaining preview format edge cases.
Details: Validate fallback behavior for unsupported codecs/containers, binary-text detection edge cases, and bounded text/code loading for very large files.

- [ ] Validate share-link policy completeness.
Details: Verify download-limit, expiration, revoke behavior, and optional password policy all match admin global settings in every public endpoint.

## 2) Reliability, testing, and CI/CD

- [ ] Add backend automated tests.
Details: Unit/integration coverage for auth/session, uploads, share permissions, range streaming, preview pipeline, and settings validation.

- [ ] Add frontend automated tests.
Details: E2E/smoke coverage for login, uploads, preview flows, share links, and admin settings flows.

- [ ] Add CI pipeline.
Details: Run `go test`, frontend build/type checks, migration checks, and Docker build smoke validation on each change.

- [ ] Validate backup/restore with disaster-recovery drills.
Details: Execute full restore in a clean environment and verify DB + storage integrity.

## 3) Operations and observability

- [ ] Standardize structured logging.
Details: Consistent app/worker logs with request ID, user ID, action, and severity.

- [ ] Add metrics and health observability.
Details: Expose metrics for upload throughput, preview queue depth, failed jobs, and API error rates.

- [ ] Define minimum alerting policy.
Details: Alerts for repeated auth failures, DB/Redis connectivity issues, elevated 5xx rates, and preview worker failures.

## 4) Production documentation

- [ ] Add an operations runbook.
Details: Deploy, update, rollback, backup, restore, secret rotation, and incident response steps.

- [ ] Finalize external reverse-proxy deployment guide.
Details: Clear examples for central Caddy/Nginx/Traefik routing to `app:3000` with required forwarded headers.

- [ ] Add compatibility matrix.
Details: Document validated Docker/Compose versions, browser behavior limits for upload/download resume, and known constraints.

- [ ] Add explicit API contract documentation.
Details: Document request/response/error models for auth, uploads (tus + custom), shares, preview endpoints, and admin endpoints.

- [ ] Add security operations documentation.
Details: Secret management, session invalidation strategy, log-retention policy, and hardening checklist for production rollout.

## 5) Cleanup already completed

- [x] Removed legacy deployment artifacts not used in current architecture:
`backend/Dockerfile`
`frontend/Dockerfile`
`docker-compose.prod.yml`
`Caddyfile`

- [x] Updated offline deployment docs to match current single-image architecture:
`docs/offline-deployment.md`
