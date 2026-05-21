# Architecture (Phase 1)

## Services

- `caddy`: reverse proxy and single entrypoint.
- `frontend`: Next.js App Router UI.
- `backend`: Go API server (Chi + pgx + Argon2id).
- `postgres`: primary datastore.
- `redis`: reserved for rate-limit/background usage.

## Backend modules

- `internal/config`: env config.
- `internal/db`: postgres connection.
- `internal/auth`: password hashing and verification.
- `internal/http`: router + handlers.
- `internal/middleware`: auth/admin middleware.
- `internal/settings`: system setting access.
- `internal/storage`: storage abstraction interface.

## Security baseline

- Argon2id password hashing.
- Random session tokens (stored as SHA-256 hash in DB).
- HttpOnly cookie sessions.
- Admin role checks on `/api/admin/*`.
- HTTP security headers (`HSTS`, `X-Frame-Options`, `nosniff`, `Permissions-Policy`, `COOP`).
- CSRF protection on mutating `/api/*` requests via Origin/Referer validation.
- Audit log entries for login and setting updates.

## Implemented settings

- `upload.max_file_size_mode`: `unlimited|custom`
- `upload.max_file_size_bytes`: nullable bigint in JSON value

## Implemented APIs (current)

- Auth: `login`, `logout`, `me`, `change-password`
- Folders: list items, create, rename, delete, move
- Files: upload, metadata, rename, delete, move, download, preview (Range)
- Upload sessions: init, chunk append, status, complete, cancel (custom resumable flow)
- Tus: create/head/patch/delete resumable uploads (`/api/uploads/tus/*`)
- Shares: create/get/list/update/delete/revoke + public share info/items/file download/file preview
- ZIP: private folder ZIP + public shared folder ZIP
- Admin: upload max file size settings (`GET/PATCH /api/admin/settings/upload`)
  plus generic/system settings, storage summary/recalculate, expired upload cleanup, audit logs
  plus user management (`/api/admin/users*`)
- Security: Redis-backed rate limiting for login and public share access
- Maintenance: periodic cleanup of expired sessions and expired upload sessions

## Next phases

- Preview pipeline + worker
- Remaining admin settings coverage and richer audit filtering UI
