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
- Audit log entries for login and setting updates.

## Implemented settings

- `upload.max_file_size_mode`: `unlimited|custom`
- `upload.max_file_size_bytes`: nullable bigint in JSON value

## Implemented APIs (current)

- Auth: `login`, `logout`, `me`
- Folders: list items, create, rename, delete, move
- Files: upload, metadata, rename, delete, move, download, preview (Range)
- Upload sessions: init, chunk append, status, complete, cancel (custom resumable flow)
- Shares: create/list/revoke + public share info/items/file download/file preview
- ZIP: private folder ZIP + public shared folder ZIP
- Admin: upload max file size settings (`GET/PATCH /api/admin/settings/upload`)
  plus generic/system settings, storage summary/recalculate, expired upload cleanup, audit logs
  plus user management (`/api/admin/users*`)
- Auth: own password change endpoint (`POST /api/auth/change-password`)

## Next phases

- Resumable upload (tus/custom chunked)
- File/folder CRUD and storage writes
- Streaming download with Range
- Preview pipeline + worker
- Share links and public routes
- Full admin panels and audit filtering
