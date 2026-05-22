# Space Operations Runbook

## Deploy

1. Prepare `.env` from `.env.example` and set production values.
2. Build and start services:
   - `docker compose build`
   - `docker compose up -d`
3. Apply migrations:
   - `./scripts/migrate.sh`
4. Verify health:
   - `docker compose ps`
   - `curl -fsS http://localhost:${SPACE_APP_PORT:-3000}/healthz`

## Update

1. Pull latest code.
2. Build images: `docker compose build`
3. Restart stack: `docker compose up -d`
4. Re-run migrations: `./scripts/migrate.sh`
5. Validate with health checks and login smoke.

## Rollback

1. Return to last known-good commit/tag.
2. Rebuild/restart:
   - `docker compose build`
   - `docker compose up -d`
3. If DB schema changed incompatibly, restore DB+storage from backup.

## Backup

- Run: `./scripts/backup.sh`
- Output: backup directory containing PostgreSQL dump + storage archive.
- Store backup artifacts off-host.

## Restore

1. Stop write traffic to the app.
2. Run restore:
   - `./scripts/restore.sh <backup_dir> --force`
3. Start services and verify:
   - `./scripts/start.sh`
   - `./scripts/migrate.sh`

## Disaster-Recovery Drill

Run the drill from a host with Docker access:

- `./scripts/drill-backup-restore.sh`

The drill starts an isolated Compose project, creates database and storage marker data, runs backup, mutates the data, restores the backup, and verifies both markers. It writes a `drill-report.txt` file into the generated backup directory.

## Secret Rotation

1. Update `.env` with new credentials/secrets.
2. Rotate DB/Redis credentials first.
3. Restart services: `docker compose up -d`
4. Validate login/session behavior.

## Session Invalidation

- Invalidate all sessions:
  - `docker compose exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c 'DELETE FROM sessions;'`
- Force password reset for affected users via admin user APIs/pages.

## Incident Response

1. Capture timeline and scope from logs (`./scripts/logs.sh`).
2. Confirm service health (`docker compose ps`, `/healthz`).
3. Contain impact (disable sharing/preview from admin settings if needed).
4. Restore from backup if data integrity is affected.
5. Document root cause and corrective actions.
