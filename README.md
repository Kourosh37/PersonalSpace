# Space

Self-hosted private file manager foundation with Docker-first deployment.

## Current status

This is phase 1 implementation (foundation):
- Dockerized stack (`app`, `preview-worker`, `postgres`, `redis`)
- Go backend with secure auth/session foundation
- Admin upload settings endpoints with DB persistence
- Next.js frontend with login, dashboard shell, admin pages
- Initial PostgreSQL migrations
- Offline image build/export/import scripts

This phase also includes an initial working file workflow:
- Folder CRUD + move
- File upload (streaming multipart) with server-side max size enforcement
- Custom resumable upload sessions (`/api/uploads/init|chunk|status|complete|cancel`)
- Tus resumable upload endpoints (`/api/uploads/tus/*`) with HEAD/PATCH/DELETE support
- File metadata/read/rename/move/delete
- File download + preview with HTTP Range support
- Folder ZIP download (private + public share)
- Share link CRUD/list/revoke
- Public share pages (`/s/{token}`) with optional password checks
- Admin APIs/pages for settings, storage summary, expired upload cleanup, and audit logs
- Admin user management APIs/pages (list/create/update/deactivate/change-password)
- Auth password change endpoint (`POST /api/auth/change-password`)
- Runtime enforcement of global sharing/preview settings on private and public routes
- Redis-backed request rate limiting for login and public share access
- Security headers middleware (`HSTS` behind HTTPS, `X-Frame-Options`, `nosniff`, `Permissions-Policy`)
- CSRF protection for state-changing API requests using Origin/Referer validation
- Background maintenance loop for expired sessions and expired upload cleanup
- Async preview worker queue for metadata previews (`preview_jobs` -> `file_previews`)

## Quick start

1. Copy env file:

```bash
cp .env.example .env
```

2. Start services:

```bash
./scripts/start.sh
```

3. Run migrations:

```bash
./scripts/migrate.sh
```

4. Create admin user:

```bash
./scripts/create-admin.sh admin strong-password-change-me
```

5. Open app:

- `http://localhost:${SPACE_APP_PORT:-3000}`
- Login with created admin credentials

## Reverse proxy note

- This project does **not** require Caddy/Nginx inside its own compose.
- Route your existing external reverse proxy to the `app` service (port `3000`).
- API routes are proxied internally by Next.js to the embedded backend service, so external proxy only needs one upstream.

## Scripts

- `scripts/build-images.sh`
- `scripts/export-images.sh`
- `scripts/import-images.sh`
- `scripts/start.sh`
- `scripts/stop.sh`
- `scripts/restart.sh`
- `scripts/logs.sh`
- `scripts/backup.sh`
- `scripts/restore.sh`
- `scripts/create-admin.sh`
- `scripts/migrate.sh`

## Offline deployment

See:
- `docs/offline-deployment.md`
- `docs/architecture.md`

## Notes

- This phase initializes production-minded structure and core modules.
- Dashboard includes Uppy Tus-based resumable upload panel.
- Office-to-PDF/media thumbnail preview generation is still a next phase.
- `backup.sh` now creates a full backup directory containing PostgreSQL dump plus storage volume archive.
- `restore.sh <backup_dir> --force` restores both DB and storage data.
