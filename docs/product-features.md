# Product Features

## File Manager

Space provides an authenticated dashboard for private file management.

Supported file manager capabilities:

- Root and nested folder browsing.
- Folder creation, rename, move, and delete.
- File upload, rename, move, delete, metadata view, preview, and download.
- Breadcrumb navigation.
- Search and sorting by name, type, size, and modified time.
- Folder ZIP download.
- File size display and modified timestamp display.
- In-app download progress panel with cancel and clear actions.

## Uploads

Space supports three upload flows.

### Multipart Upload

Use this for normal file uploads from the dashboard. The backend streams file data to storage and enforces:

- Admin upload max file size policy.
- Per-user storage quota.
- Duplicate name constraints in folders.
- Server-side checksum and MIME sniffing.

### Custom Resumable Uploads

The custom resumable API supports:

- Create upload session.
- Append chunks at an expected offset.
- Query upload status.
- Complete upload into a file record.
- Cancel upload session.

This API is useful for custom clients that need explicit session control.

### Tus Resumable Uploads

The dashboard uses Uppy with Tus for resumable browser uploads.

Supported Tus behavior:

- `POST` create upload.
- `HEAD` query offset.
- `PATCH` append bytes.
- `DELETE` cancel upload.
- Persistent Tus fingerprinting for browser-supported resume.
- Retry handling and status display.
- Folder upload selection on browsers with directory picker APIs.

Browser limitation: if the tab is suspended or closed, active streams may pause. Users can resume where supported by reselecting or continuing the same file.

## Folder Upload

Folder upload recursively queues browser-selected files. Current behavior stores files in the selected Space destination folder and encodes nested path segments into filenames to reduce collisions.

Example:

```text
Invoices/2026/May/report.pdf -> Invoices__2026__May__report.pdf
```

## Preview System

Space supports direct and generated previews.

Direct preview categories:

- Images.
- Audio.
- Video.
- PDF.
- Text/code files.
- CSV files with delimiter detection and row limits.

Generated preview categories:

- Metadata JSON.
- Image thumbnails.
- Video thumbnails through `ffmpeg`.
- Office-to-PDF through LibreOffice.
- Audio/video metadata through `ffprobe`.

Preview safety:

- Risky inline formats such as HTML, SVG, XML, and XHTML are blocked from unsafe inline rendering.
- Text and CSV previews are bounded by configured byte/row limits.
- Public previews respect both sharing settings and preview settings.

## Sharing

Share links can target files or folders.

Share controls:

- Optional password.
- Optional expiration.
- Optional maximum downloads.
- Allow/deny preview.
- Allow/deny download.
- Allow/deny folder browsing.
- Revoke.

Public share behavior:

- File shares can preview and download when policy allows.
- Folder shares can list items when folder browsing is allowed.
- Folder shares can download ZIP archives when downloads are allowed.
- Password-protected shares require password verification.
- Expired, revoked, over-limit, and globally-disabled shares are denied.

## Administration

Admin capabilities:

- User list, create, update, deactivate, quota management, and password reset.
- Upload settings.
- Sharing settings.
- Preview settings.
- Generic system settings.
- Storage summary and storage usage recalculation.
- Expired upload cleanup.
- Preview cache cleanup.
- Audit log browsing.
- System info and health pages.

## Security And Observability

Security and operations capabilities:

- Argon2id password hashing.
- SHA-256 hashed session tokens.
- HttpOnly cookies.
- Configurable Secure and SameSite cookie policy.
- CSRF protection by Origin/Referer validation.
- Security headers.
- Redis-backed rate limiting.
- Audit logs for security and administrative actions.
- Structured JSON logs.
- Prometheus-compatible metrics.
- Health endpoint.
- Backup/restore scripts.
- Disaster-recovery drill script.
