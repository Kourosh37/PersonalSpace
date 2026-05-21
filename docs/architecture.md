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

## Next phases

- Resumable upload (tus/custom chunked)
- File/folder CRUD and storage writes
- Streaming download with Range
- Preview pipeline + worker
- Share links and public routes
- Full admin panels and audit filtering