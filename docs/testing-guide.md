# Testing Guide

## Backend Unit Tests

Run:

```bash
cd backend
go test ./...
```

Covered areas include:

- Password hashing and verification.
- Upload settings validation.
- Sharing and preview setting validation.
- Tus metadata parsing.
- HTTP range parsing.
- Preview classification and policy helpers.
- Folder helper behavior.
- Preview image resizing and video detection.
- Metrics output.

## Backend Integration Tests

Integration tests are guarded because they reset the target database.

Required environment:

```bash
export INTEGRATION_DB_DSN='postgres://space:space@127.0.0.1:5432/space_test?sslmode=disable'
export SPACE_TEST_ALLOW_DB_RESET=1
cd backend
go test ./internal/http
```

Integration coverage includes:

- Login/session flow.
- Authenticated `/me`.
- Multipart upload persistence.
- Quota rejection.
- Share policy enforcement.
- Preview job persistence.

Never point `INTEGRATION_DB_DSN` at production.

## Frontend Type Check

Run:

```bash
cd frontend
npm run typecheck
```

## Frontend Build

Run:

```bash
cd frontend
npm run build
```

The project uses Next.js standalone output. Build must succeed before production image creation and Playwright E2E tests.

## Frontend E2E Smoke Tests

Run:

```bash
cd frontend
npm run test:e2e
```

The Playwright suite starts the standalone Next server and mocks API responses. It covers:

- Login page render and submit behavior.
- Dashboard file manager smoke path.
- Admin upload settings page.
- Admin users page.
- Public share page.

If Playwright browsers are missing:

```bash
cd frontend
npx playwright install chromium
```

## Full Local Verification

Recommended before committing production changes:

```bash
cd backend
go test ./...

cd ../frontend
npm run build
npm run typecheck
npm run test:e2e

cd ..
bash -n scripts/lib/compose.sh scripts/backup.sh scripts/restore.sh scripts/drill-backup-restore.sh
./scripts/drill-backup-restore.sh
```

## CI

GitHub Actions runs:

- Go vet.
- Go tests with a PostgreSQL service for integration tests.
- Frontend dependency install.
- Type check.
- Next.js build.
- Playwright Chromium smoke tests.
- Docker image build smoke.
- Compose migration smoke.

## Docker Build Verification

Run:

```bash
docker build -t space-app:local-verify .
```

If this fails during `apk add`, fix Docker network/proxy/mirror access. That failure is environmental when tests and application builds pass outside Docker.
