# Troubleshooting

## Docker Build Fails During `apk add`

Symptoms:

```text
WARNING: fetching APKINDEX.tar.gz: TLS: unspecified error
ERROR: unable to select packages
```

Likely causes:

- Docker daemon cannot reach Alpine mirrors.
- Host proxy is not available inside Docker or WSL.
- TLS interception is blocking package index fetches.
- Temporary Alpine mirror outage.

Fixes:

- Configure Docker Desktop proxy settings.
- Use a network where Docker can reach `dl-cdn.alpinelinux.org`.
- Retry from the deployment server.
- Prebuild and export the image from a network that can reach package mirrors.

## Docker Build Fails During `go mod download` Or `npm ci`

Symptoms:

```text
go mod download ... proxy.golang.org ... EOF
npm ci ... network timeout
```

Likely causes:

- Docker build containers do not have the same proxy/network access as the host.
- Go/NPM registries are blocked or intermittently unreachable.
- WSL/Docker Desktop proxy settings are incomplete.

Fixes:

- Configure Docker daemon proxy settings, not only host shell proxy variables.
- Retry on the deployment server.
- Prebuild images in a network that can access Go, NPM, Docker, and Alpine registries.
- For restricted networks, use `docs/offline-deployment.md` to export/import images.

## Login Fails In Production HTTPS

Check:

- `PUBLIC_BASE_URL=https://...`
- `BACKEND_SESSION_SECURE=true`
- `BACKEND_ENFORCE_SECURE_COOKIES=true`
- Reverse proxy sends `X-Forwarded-Proto=https`.
- Browser receives the session cookie.

If `BACKEND_SESSION_SAME_SITE=none`, Secure cookies are mandatory.

## CSRF Errors

Symptoms:

```json
{ "error": "request blocked by csrf protection" }
```

Check:

- `BACKEND_ALLOWED_ORIGINS` includes the exact public origin.
- Reverse proxy preserves `Host`.
- Browser requests include correct `Origin` or `Referer`.
- `PUBLIC_BASE_URL` is correct.

Do not disable CSRF in production except for a temporary emergency mitigation with compensating controls.

## Upload Exceeds Quota

Possible causes:

- User `storage_quota_bytes` is lower than current usage plus new file size.
- Storage usage drift requires admin recalculation.
- A previous upload completed but the UI has stale state.

Fix:

- Check admin user quota.
- Recalculate storage usage from admin storage page.
- Remove old files or increase quota.

## Tus Upload Does Not Resume

Resume depends on:

- Same browser profile.
- Same file fingerprint.
- Tus session still present and not expired.
- Browser did not clear site storage.
- Network/proxy supports PATCH requests.

Check upload session status in the database or admin cleanup view.

## Public Share Does Not Open

Possible causes:

- Share revoked.
- Share expired.
- Max downloads reached.
- Password required or wrong password.
- Global sharing disabled.
- Public preview/download disabled.

Check admin sharing settings and the share record.

## Preview Does Not Generate

Check:

- `preview-worker` container is running.
- `preview.enabled` is true.
- Relevant preview setting is enabled, such as office/media/thumbnail.
- `/metrics` preview queue values.
- Worker logs for LibreOffice or ffmpeg failures.
- File size is within preview generation policy.

Office previews require LibreOffice in the image. Video thumbnails and media metadata require ffmpeg/ffprobe.

## `/metrics` Is Empty Or Missing DB Gauges

Runtime counters start at process boot. DB-backed gauges require database connectivity.

Check:

- App can connect to PostgreSQL.
- `/healthz` succeeds.
- Logs do not show DB errors.

## Backup Restore Fails

Check:

- Backup directory contains `db.sql`, `storage.tar.gz`, and `manifest.txt`.
- PostgreSQL service is healthy.
- `restore.sh` was called with `--force`.
- Docker volume exists or can be created.

Run the drill:

```bash
./scripts/drill-backup-restore.sh
```

## Reverse Proxy Returns 502

Check:

- `app` service is healthy.
- Reverse proxy can resolve the target container or host port.
- Target is port `3000`, not backend port `8080`.
- If using a separate Compose project for proxy, the proxy is attached to the correct Docker network or uses the host-published port.

## Static Assets Missing In Standalone Next Server

If running frontend standalone manually, ensure `.next/static` is copied into `.next/standalone/.next/static`.

The repo's Playwright helper handles this through:

```bash
cd frontend
npm run start:standalone
```
