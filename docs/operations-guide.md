# Operations Guide

## Health Checks

Public health endpoint:

```bash
curl -fsS http://127.0.0.1:${SPACE_APP_PORT:-3000}/healthz
```

Expected response:

```json
{ "status": "ok" }
```

Docker Compose also defines health checks for `app`, `postgres`, and `redis`.

## Metrics

Metrics endpoint:

```bash
curl -fsS http://127.0.0.1:${SPACE_APP_PORT:-3000}/metrics
```

Metrics are Prometheus text format.

Important metrics:

- `space_http_requests_total`
- `space_http_request_duration_ms_total`
- `space_uploaded_bytes_total`
- `space_preview_job_failures_total`
- `space_preview_jobs{status="queued"}`
- `space_preview_jobs{status="processing"}`
- `space_preview_jobs{status="failed"}`
- `space_active_upload_sessions`
- `space_active_upload_bytes`

## Logs

Backend processes use structured JSON logs through `slog`.

Service labels:

- `space-server`
- `space-preview-worker`
- `space-migrate`
- `space-create-admin`

Useful command:

```bash
./scripts/logs.sh
```

Set `LOG_LEVEL=debug`, `info`, `warn`, or `error` when deeper diagnostics are needed.

## Backups

Run:

```bash
./scripts/backup.sh
```

Backup output:

```text
backups/space-backup-YYYYMMDD-HHMMSS/
  db.sql
  storage.tar.gz
  manifest.txt
```

The database dump includes clean/drop statements. Storage is archived from the Space storage Docker volume.

## Restore

Restore is destructive.

Run:

```bash
./scripts/restore.sh backups/space-backup-YYYYMMDD-HHMMSS --force
```

Restore behavior:

- Ensures PostgreSQL is running.
- Restores `db.sql` with `ON_ERROR_STOP=1`.
- Replaces storage volume content from `storage.tar.gz`.

After restore:

```bash
./scripts/start.sh
curl -fsS http://127.0.0.1:${SPACE_APP_PORT:-3000}/healthz
```

## Disaster-Recovery Drill

Run:

```bash
./scripts/drill-backup-restore.sh
```

The drill:

- Starts an isolated Compose project.
- Applies migrations.
- Creates a database marker and storage marker.
- Runs backup.
- Deletes the markers.
- Runs restore.
- Verifies both markers.
- Writes `drill-report.txt` into the generated backup directory.

Keep drill reports as operational evidence.

## Maintenance

The server starts a maintenance loop that periodically:

- Removes expired sessions.
- Marks expired upload sessions.
- Cleans up expired upload temp data where possible.

Admin APIs also expose cleanup actions for expired uploads and preview cache.

## Incident Response

Minimum response flow:

1. Confirm service health.
2. Check structured logs.
3. Check PostgreSQL and Redis health.
4. Check `/metrics` for 5xx rates, upload backlog, and preview queue backlog.
5. Stop write traffic if data integrity is at risk.
6. Restore from backup if needed.
7. Document timeline, root cause, and corrective action.

## Alerting

Use `docs/alerting-policy.md` as the minimum policy.

Page immediately for:

- `/healthz` failures.
- PostgreSQL outage.
- Repeated admin login failures.
- Sustained 5xx errors.

Create high-priority tickets for:

- Redis outage.
- Preview queue backlog.
- Upload backlog.
- Backup failures.
- Restore drill overdue.
