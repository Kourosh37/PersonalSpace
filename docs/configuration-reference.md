# Configuration Reference

Space is configured through environment variables. In Docker Compose, values come from `.env` and are passed to the containers.

## Application

| Variable | Default | Description |
| --- | --- | --- |
| `APP_NAME` | `Space` | Display/application name. |
| `PUBLIC_BASE_URL` | `http://localhost` | Public URL used for share links and security validation. |
| `SPACE_APP_PORT` | `3000` | Host port mapped to the app container's port `3000`. |
| `BACKEND_HTTP_ADDR` | `:8080` | Internal backend HTTP listener inside the app container. |
| `BACKEND_STORAGE_ROOT` | `/data/storage` | Storage root mounted from the Docker volume. |

## Database

| Variable | Default | Description |
| --- | --- | --- |
| `POSTGRES_DB` | `space` | Database name. |
| `POSTGRES_USER` | `space` | Database user. |
| `POSTGRES_PASSWORD` | `space` | Database password. Change in production. |
| `POSTGRES_HOST` | `postgres` | Compose service hostname. |
| `POSTGRES_PORT` | `5432` | PostgreSQL port. |
| `DB_DSN` | constructed in Compose | Backend connection string. |

## Redis

| Variable | Default | Description |
| --- | --- | --- |
| `REDIS_ADDR` | `redis:6379` | Redis address used by the rate limiter. |

If Redis is unavailable, the app logs a warning and continues without strict Redis-backed limiting.

## Session And Cookie Security

| Variable | Default | Description |
| --- | --- | --- |
| `BACKEND_SESSION_COOKIE_NAME` | `space_session` | Session cookie name. |
| `BACKEND_SESSION_TTL_HOURS` | `168` | Session lifetime. |
| `BACKEND_SESSION_SECURE` | `false` | Sets Secure cookie flag. Must be `true` for HTTPS production. |
| `BACKEND_SESSION_SAME_SITE` | `lax` | `lax`, `strict`, or `none`. |
| `BACKEND_ENFORCE_SECURE_COOKIES` | `true` | Requires secure cookies when `PUBLIC_BASE_URL` is HTTPS. |

Rules:

- If `PUBLIC_BASE_URL` starts with `https://` and enforcement is enabled, `BACKEND_SESSION_SECURE=true` is required.
- If `BACKEND_SESSION_SAME_SITE=none`, `BACKEND_SESSION_SECURE=true` is required.

## CSRF And Origins

| Variable | Default | Description |
| --- | --- | --- |
| `BACKEND_CSRF_DISABLED` | `false` | Disables CSRF protection only when set to `true`. Keep false in production. |
| `BACKEND_ALLOWED_ORIGINS` | empty | Comma-separated trusted origins. Include the production HTTPS URL. |

CSRF validation applies to state-changing `/api/*` requests when a session cookie is present. It checks `Origin`, then `Referer`.

## Rate Limits

All values are per minute.

| Variable | Default | Description |
| --- | --- | --- |
| `SECURITY_LOGIN_RATE_LIMIT_PER_MINUTE` | `15` | Login attempts per IP. |
| `SECURITY_SHARE_PASSWORD_RATE_LIMIT_PER_MINUTE` | `20` | Public share password attempts per IP/share. |
| `SECURITY_UPLOAD_INIT_RATE_LIMIT_PER_MINUTE` | `60` | Custom upload session creation. |
| `SECURITY_UPLOAD_COMPLETE_RATE_LIMIT_PER_MINUTE` | `60` | Custom upload completion. |
| `SECURITY_TUS_CREATE_RATE_LIMIT_PER_MINUTE` | `60` | Tus upload creation. |
| `SECURITY_PREVIEW_JOB_RATE_LIMIT_PER_MINUTE` | `30` | Preview job creation. |
| `SECURITY_ZIP_DOWNLOAD_RATE_LIMIT_PER_MINUTE` | `20` | Folder ZIP download. |

## Preview Worker

| Variable | Default | Description |
| --- | --- | --- |
| `PREVIEW_WORKER_POLL_INTERVAL_SECONDS` | `3` | Poll interval for queued preview jobs. |
| `PREVIEW_WORKER_MAX_ATTEMPTS` | `3` | Maximum processing attempts before marking a job failed. |

## Runtime System Settings

Runtime settings are stored in PostgreSQL in `system_settings`.

Important keys:

- `upload.max_file_size_mode`
- `upload.max_file_size_bytes`
- `sharing.enabled`
- `sharing.public_preview_enabled`
- `sharing.public_download_enabled`
- `sharing.allow_folder_sharing`
- `sharing.allow_permanent_links`
- `sharing.require_password_mode`
- `sharing.default_expiration_hours`
- `preview.enabled`
- `preview.public_preview_enabled`
- `preview.office_enabled`
- `preview.media_enabled`
- `preview.image_thumbnails_enabled`
- `preview.text_max_bytes`
- `preview.csv_max_rows`
- `preview.office_conversion_timeout_seconds`
- `preview.max_auto_generate_size_bytes`
