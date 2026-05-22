# TODO - Space Production Completion

This file tracks all remaining work needed to make the project fully production-ready based on the current codebase state.

## 1) Product feature gaps

- [x] Complete dashboard file manager parity.
Details: Ensure production-grade behavior for breadcrumb navigation, grid/list switch, details panel, and polish for large directories.

- [x] Complete admin settings coverage beyond upload/sharing/preview.
Details: Add complete `general`, `download`, and `security` sections with backend validation + audit logging.

- [x] Fully enforce per-user storage quota.
Details: Enforce quota consistently across multipart, tus, and custom resumable uploads.

- [x] Complete advanced upload settings.
Details: Support and enforce max concurrent uploads, upload session expiration policy, and optional blocklists (default allow-all).

- [x] Improve public share UX parity.
Details: Better folder breadcrumb/navigation, clearer state/error UX, and full preview behavior parity with private dashboard.

- [x] Complete remaining preview format edge cases.
Details: Validate fallback behavior for unsupported codecs/containers, binary-text detection edge cases, and bounded text/code loading for very large files.

- [x] Validate share-link policy completeness.
Details: Verify download-limit, expiration, revoke behavior, and optional password policy all match admin global settings in every public endpoint.

## 2) Reliability, testing, and CI/CD

- [ ] Add backend automated tests.
Details: Current baseline tests were added for `settings_validation` and HTTP range parsing. Remaining: auth/session, uploads, share permissions, preview pipeline, and DB-backed integration coverage.

- [ ] Add frontend automated tests.
Details: E2E/smoke coverage for login, uploads, preview flows, share links, and admin settings flows.

- [x] Add CI pipeline.
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

Completed and removed from active TODO:
- operations runbook
- external reverse-proxy deployment guide
- compatibility matrix
- explicit API contract documentation
- security operations documentation

## 5) Cleanup already completed

- [x] Removed legacy deployment artifacts not used in current architecture:
`backend/Dockerfile`
`frontend/Dockerfile`
`docker-compose.prod.yml`
`Caddyfile`

- [x] Updated offline deployment docs to match current single-image architecture:
`docs/offline-deployment.md`
