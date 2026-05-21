# Space

Self-hosted private file manager foundation with Docker-first deployment.

## Current status

This is phase 1 implementation (foundation):
- Dockerized stack (`caddy`, `frontend`, `backend`, `postgres`, `redis`)
- Go backend with secure auth/session foundation
- Admin upload settings endpoints with DB persistence
- Next.js frontend with login, dashboard shell, admin upload settings page
- Initial PostgreSQL migrations
- Offline image build/export/import scripts

This phase also includes an initial working file workflow:
- Folder CRUD + move
- File upload (streaming multipart) with server-side max size enforcement
- File metadata/read/rename/move/delete
- File download + preview with HTTP Range support
- Share link creation/revoke/list
- Public share pages (`/s/{token}`) with optional password checks

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

- `http://localhost`
- Login with created admin credentials

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
- Tus resumable upload, ZIP folder download, office/media preview workers, and advanced admin settings are next.
