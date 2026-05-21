# Offline Deployment

## On build machine (online)

```bash
./scripts/build-images.sh
./scripts/export-images.sh
```

Bundle these files/directories for transfer:
- `docker-compose.yml`
- `docker-compose.prod.yml`
- `Caddyfile`
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

## Backup and restore

Create full backup (database + storage volume):

```bash
./scripts/backup.sh
```

Restore from a backup directory (destructive):

```bash
./scripts/restore.sh backups/space-backup-YYYYMMDD-HHMMSS --force
```
