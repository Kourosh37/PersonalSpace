# Offline Deployment

## On build machine (online)

```bash
./scripts/build-images.sh
./scripts/export-images.sh
```

Bundle these files/directories for transfer:
- `docker-compose.yml`
- `docker-compose.prod.yml`
- `Caddyfile` (optional, only if you want a dedicated proxy for this project)
- `.env` (or `.env.example` then create `.env` on target)
- `scripts/`
- `backend/migrations/`
- `images/`
- `README.md`
- `docs/`

## On target server (offline)

```bash
./scripts/import-images.sh
./scripts/start.sh
./scripts/migrate.sh
./scripts/create-admin.sh admin your-strong-password
```

No internet image pulls are required after import.

If you already have a reverse proxy on the server, point it to:
- `space app container` on port `3000` (or your configured `SPACE_APP_PORT` on host)
- No Caddy container inside this project is required.

## Backup and restore

Create full backup (database + storage volume):

```bash
./scripts/backup.sh
```

Restore from a backup directory (destructive):

```bash
./scripts/restore.sh backups/space-backup-YYYYMMDD-HHMMSS --force
```
