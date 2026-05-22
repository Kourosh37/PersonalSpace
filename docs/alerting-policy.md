# Minimum Alerting Policy

This policy defines the minimum production alerts for Space when logs and `/metrics` are shipped to the monitoring stack.

## Required Signals

- `/healthz` availability from the reverse proxy or monitoring network.
- `/metrics` Prometheus scrape from the application container.
- Structured application logs from `space-server`, `space-preview-worker`, `space-migrate`, and `space-create-admin`.
- PostgreSQL and Redis container health from Docker or the host monitoring agent.

## Alerts

### Service Availability

- `SpaceDown`: `/healthz` fails for 2 consecutive checks over 2 minutes.
- `MetricsScrapeFailed`: `/metrics` cannot be scraped for 5 minutes.

### API Errors

- `Elevated5xxRate`: `space_http_requests_total{status_class="5xx"}` increases by more than 10 requests in 5 minutes.
- `SustainedAPIWarnings`: `space_http_requests_total{status_class="4xx"}` increases sharply above the expected baseline for 15 minutes. Tune the threshold after production traffic is known.

### Authentication Abuse

- `RepeatedAuthFailures`: structured logs contain `auth.login.failed` more than 20 times from the same IP in 10 minutes.
- `AdminAuthFailures`: any repeated `auth.login.failed` event for a known admin username should page the operator after 5 failures in 10 minutes.

### Dependencies

- `DatabaseUnavailable`: health endpoint or logs report PostgreSQL connectivity failures for 2 minutes.
- `RedisUnavailable`: logs contain `redis rate limiter ping failed` or Redis health checks fail for 5 minutes.

### Uploads

- `UploadFailureSpike`: resumable upload sessions in `failed` status increase materially over 10 minutes.
- `ActiveUploadBacklog`: `space_active_upload_sessions` remains above the expected operational threshold for 30 minutes.

### Preview Worker

- `PreviewQueueBacklog`: `space_preview_jobs{status="queued"}` stays above the expected worker capacity for 30 minutes.
- `PreviewProcessingStuck`: `space_preview_jobs{status="processing"}` remains non-zero without completions for 30 minutes.
- `PreviewFailureSpike`: `space_preview_job_failures_total` increases by more than 10 in 15 minutes.

### Backup And Restore

- `BackupMissing`: no successful backup artifact is produced within the configured backup interval.
- `RestoreDrillOverdue`: no documented restore drill has been completed in the last 30 days.

## Response Rules

- Page immediately for service availability, database outage, repeated admin authentication failures, and sustained 5xx errors.
- Create high-priority tickets for preview backlog, Redis outage, upload backlog, and backup failures.
- Record every page-triggering incident in the operations runbook with timeline, scope, root cause, and corrective action.
