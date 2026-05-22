# Deployment Guide

## Deployment Model

Space is deployed with Docker Compose.

Services:

- `app`: one image containing Next.js frontend and Go backend.
- `preview-worker`: same image as `app`, but starts the preview worker binary.
- `postgres`: PostgreSQL 16.
- `redis`: Redis 7.

No Caddy, Nginx, or Traefik service is included in the Space stack. Use your existing external reverse proxy and route to `app:3000`.

## Directory Layout On Server

Recommended layout:

```text
/opt/deployed_projects/space
  .env
  docker-compose.yml
  scripts/
  docs/
```

If you keep a central reverse proxy in another directory, attach it to the Docker network that can resolve the Space `app` container, or route through a host-published port.

## First Deployment

1. Create `.env` from `.env.example`.
2. Set production secrets and URLs.
3. Build the image:

```bash
docker compose build
```

4. Start PostgreSQL and Redis:

```bash
docker compose up -d postgres redis
```

5. Run migrations:

```bash
./scripts/migrate.sh
```

6. Start the application:

```bash
./scripts/start.sh
```

7. Create the first admin:

```bash
./scripts/create-admin.sh admin 'change-this-password'
```

8. Verify:

```bash
curl -fsS http://127.0.0.1:${SPACE_APP_PORT:-3000}/healthz
```

## Production HTTPS

Set these values in `.env`:

```env
PUBLIC_BASE_URL=https://space.example.com
BACKEND_SESSION_SECURE=true
BACKEND_ENFORCE_SECURE_COOKIES=true
BACKEND_CSRF_DISABLED=false
BACKEND_ALLOWED_ORIGINS=https://space.example.com
```

The external reverse proxy must send:

- `Host`
- `X-Forwarded-For`
- `X-Forwarded-Proto=https`

## Updating

1. Pull the new code or import the new image.
2. Build or load image.
3. Run migrations.
4. Restart services.
5. Verify health, login, dashboard, upload, and share smoke flows.

Commands:

```bash
docker compose build
./scripts/migrate.sh
./scripts/restart.sh
curl -fsS http://127.0.0.1:${SPACE_APP_PORT:-3000}/healthz
```

## Rollback

Rollback requires the last known-good application image and, if migrations are incompatible, a backup.

Recommended rollback sequence:

1. Stop write traffic at the reverse proxy.
2. Restore the previous image or Git tag.
3. If DB compatibility is uncertain, restore the last backup.
4. Start services.
5. Verify health and login.

## Volumes

Persistent volumes:

- `space_postgres`: PostgreSQL data.
- `space_storage`: file storage, upload temp data, and generated previews.

Do not delete these volumes unless intentionally destroying the environment or restoring from backup.

## Image Build Notes

The runtime image installs LibreOffice, ffmpeg, and fonts so preview features do not depend on host packages.

If Docker build fails during `apk add`, the usual causes are:

- Docker daemon cannot reach Alpine package mirrors.
- Corporate/local proxy is configured on the host but not available inside Docker/WSL.
- TLS interception or mirror outage.

Fix Docker network/proxy configuration or build on the target server with working package mirror access.
