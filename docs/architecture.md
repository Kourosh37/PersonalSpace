# Architecture

## High-Level Model

Space is a Docker-first application composed of:

- A single production application image.
- PostgreSQL for durable relational state.
- Redis for rate limiting.
- A Docker volume for file and preview storage.
- An external reverse proxy managed outside this project.

The application image contains:

- Go backend binaries.
- Next.js standalone frontend server.
- Migration CLI.
- Admin creation CLI.
- Preview worker CLI.
- LibreOffice and ffmpeg tooling for preview generation.

## Runtime Services

### `app`

The `app` service runs `/app/bin/start-app.sh`, which starts:

- Go backend on `127.0.0.1:8080` inside the container.
- Next.js standalone server on `0.0.0.0:3000`.

External traffic should reach only port `3000`.

Next.js rewrites:

- `/api/*` -> `http://127.0.0.1:8080/api/*`
- `/healthz` -> `http://127.0.0.1:8080/healthz`

### `preview-worker`

The `preview-worker` service uses the same image and starts `/app/bin/preview-worker`.

It polls `preview_jobs`, reads source files from storage, writes generated previews to storage, and records `file_previews`.

### `postgres`

PostgreSQL stores:

- Users.
- Sessions.
- Folders.
- Files.
- Upload sessions.
- Share links.
- Preview jobs.
- File previews.
- System settings.
- Audit logs.
- Migration state.

### `redis`

Redis stores rate-limit counters. If Redis is unavailable, the backend logs a warning and continues without strict Redis-backed limiting.

## Backend Modules

- `internal/config`: environment parsing and safety validation.
- `internal/db`: PostgreSQL connection and migration guard.
- `internal/auth`: Argon2id password hashing and random session token helpers.
- `internal/http`: API routes and handlers.
- `internal/middleware`: auth, admin, CSRF, security headers, and structured request logging.
- `internal/settings`: upload setting persistence and validation.
- `internal/storage`: storage abstraction and local Docker-volume implementation.
- `internal/preview`: asynchronous preview worker.
- `internal/maintenance`: expired session/upload cleanup.
- `internal/security`: Redis-backed rate limiter.
- `internal/observability`: metrics counters and Prometheus output.
- `internal/logging`: structured JSON logging setup.

## Data Flow

### Login

1. User submits username/password.
2. Backend verifies Argon2id password hash.
3. Backend creates a random session token.
4. SHA-256 token hash is stored in `sessions`.
5. Raw token is returned as an HttpOnly cookie.

### Authenticated Request

1. Middleware reads session cookie.
2. Cookie token is hashed.
3. Active session and active user are loaded from PostgreSQL.
4. User is attached to request context.
5. Admin middleware checks role for `/api/admin/*`.

### Multipart Upload

1. Backend streams multipart file into temp storage.
2. File size policy is enforced during streaming.
3. Storage object is moved to final key.
4. File metadata is inserted.
5. User storage usage is updated under a transaction.
6. Quota is checked under row lock.

### Resumable Upload

1. Client creates upload session.
2. Chunks are appended at expected offsets.
3. Client completes the session.
4. Backend moves temp object to final storage key.
5. File metadata and user usage are committed.

### Share Access

1. Public token is hashed and resolved.
2. Global sharing settings are checked.
3. Expiration, revoke state, password, and download limits are checked.
4. Target file/folder permissions are enforced.
5. Preview/download/list response is returned when allowed.

### Preview Job

1. User or system creates preview job.
2. Worker claims queued job.
3. Worker reads source file from storage.
4. Worker generates metadata, thumbnail, or PDF.
5. Worker writes preview object to storage.
6. Worker upserts `file_previews` and completes job.

## Security Boundaries

- Frontend routing is not a security boundary.
- Backend middleware enforces session and admin checks.
- Storage files are not directly exposed by the reverse proxy.
- Public share APIs resolve and validate share policy on every request.
- CSRF protection is enforced for mutating API requests with session cookies.
- Risky inline preview formats are blocked.

## Observability

Runtime observability includes:

- `/healthz` for service health.
- `/metrics` for Prometheus-compatible metrics.
- Structured JSON logs with request ID, status, duration, method, path, and service labels.
- Audit logs in PostgreSQL for security-sensitive events.

## Deployment Boundary

The project intentionally excludes an internal Caddy/Nginx service. Production deployments should use an external reverse proxy already managed by the host operator.
