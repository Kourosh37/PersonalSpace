# Security Threat Model (Phase 1)

This document defines the core security threats considered in the current Space architecture and the implemented controls.

## Scope

- Public endpoints (`/api/public/*`)
- Authenticated API endpoints (`/api/*`)
- Upload/preview/download pipeline
- Session-based authentication
- Database-backed metadata and local object storage

## Trust boundaries

- Browser/client to reverse proxy
- Reverse proxy to Space app container
- Space app to PostgreSQL
- Space app to Redis
- Space app to local storage volume

## Threats and controls

### 1) Brute-force login attempts

Threat:
- High-volume password guessing against `/api/auth/login`.

Controls:
- Redis-backed per-IP rate limiting on auth login.
- Generic auth failure response (`invalid credentials`).
- Audit logging of login failures.

Residual risk:
- Distributed botnets across many IPs can still cause noisy attempts.

### 2) Share token guessing and abuse

Threat:
- Enumerating public share tokens and repeated password attempts.

Controls:
- Cryptographically strong random share tokens (stored hashed in DB).
- Public share password verification rate limiting.
- Public share access rate limiting.
- Share revoke/expiry/max-download checks.
- Admin global sharing/preview/download policy enforcement.

Residual risk:
- Very high distributed traffic can still create load; edge WAF/rate-limit is recommended.

### 3) Path traversal and storage key injection

Threat:
- User-controlled paths escaping storage root or zip paths.

Controls:
- Internal generated storage keys; no direct filesystem path exposure.
- Folder/file names normalized.
- ZIP entries normalized and traversal-protected (`../` blocked).
- Folder tree ownership checks on all private/public traversal queries.

Residual risk:
- Malicious filenames may still be visually confusing; UI-level escaping remains important.

### 4) Malicious file payloads / active content execution

Threat:
- Uploaded active content (HTML/SVG/XML) executing in browser during preview.

Controls:
- Inline preview blocked for risky active content types (`html`, `svg`, `xml`, `xhtml`).
- Safe text/CSV preview via JSON payload endpoints (`preview-content`).
- `X-Content-Type-Options: nosniff` security header.
- CSP sandbox header on inline preview responses.

Residual risk:
- Downloaded files opened locally are outside browser sandbox and depend on client safety posture.

### 5) Content sniffing and response-type confusion

Threat:
- Browser interpreting bytes as executable content due to sniffing.

Controls:
- `nosniff` header on responses via security middleware.
- Explicit `Content-Type`, `Content-Disposition`, and range headers on file streams.
- Strict handling for risky inline preview paths.

Residual risk:
- Upstream proxies should not rewrite security headers.

### 6) Session theft or weak session transport

Threat:
- Session cookie leakage on non-HTTPS deployments.

Controls:
- HttpOnly session cookies.
- Configurable SameSite policy.
- Startup guard: when `PUBLIC_BASE_URL` is HTTPS and secure-cookie enforcement is enabled, app refuses to start unless `BACKEND_SESSION_SECURE=true`.
- `SameSite=None` blocked unless `Secure=true`.

Residual risk:
- If reverse proxy is misconfigured and forwards plain HTTP externally, transport security is weakened.

### 7) Privilege bypass (IDOR / authz flaws)

Threat:
- Accessing files/folders/shares not owned by requester.

Controls:
- Owner-scoped queries on private routes.
- Role checks on admin routes.
- Share-boundary checks for public folder traversal and public file access.

Residual risk:
- Requires continued endpoint-by-endpoint review as features expand.

## Operational recommendations

- Terminate TLS at reverse proxy and forward `X-Forwarded-Proto=https`.
- Keep `BACKEND_ENFORCE_SECURE_COOKIES=true` in production.
- Keep CSRF enabled (`BACKEND_CSRF_DISABLED=false`) in production.
- Rotate admin credentials and monitor audit logs.
- Apply edge-level rate limiting/WAF in front of Space.
