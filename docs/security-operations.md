# Security Operations

## Secret Management

- Keep `.env` out of source control.
- Use strong values for DB credentials and session settings.
- Rotate secrets on schedule and after incidents.

## Session Strategy

- Session cookies are server-backed through the `sessions` table.
- For global invalidation, delete session rows.
- Enforce secure cookies and HTTPS in production:
  - `BACKEND_ENFORCE_SECURE_COOKIES=true`
  - `BACKEND_SESSION_SECURE=true`

## Log Retention

- Retain app and audit logs per policy, for example 30-90 days.
- Export or ship logs to central storage when available.
- Avoid logging sensitive payloads, passwords, session tokens, or share passwords.

## Alerting

- Use `docs/alerting-policy.md` as the minimum alert baseline.
- Page on service downtime, PostgreSQL outage, repeated admin login failures, and sustained 5xx errors.
- Review thresholds after real production traffic establishes a baseline.

## Hardening Checklist

- Set `PUBLIC_BASE_URL` to the HTTPS URL.
- Ensure the reverse proxy sends `X-Forwarded-Proto=https`.
- Keep CSRF protection enabled with `BACKEND_CSRF_DISABLED=false`.
- Restrict `BACKEND_ALLOWED_ORIGINS` to trusted origins.
- Enforce least-privilege admin access.
- Run periodic restore drills using backup artifacts.
