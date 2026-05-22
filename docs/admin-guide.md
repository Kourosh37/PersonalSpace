# Admin Guide

## Admin Access

Admin users can access `/admin` pages after login. Admin-only APIs are under `/api/admin/*`.

Admin role checks are enforced by backend middleware. Hiding frontend navigation is not the security boundary.

## User Management

Admins can:

- List users.
- Create users.
- Assign `admin` or `user` role.
- Activate or deactivate accounts.
- Set per-user storage quota.
- Clear per-user storage quota.
- Reset user passwords.

Quota behavior:

- `storage_quota_bytes = NULL` means unlimited.
- Uploads are rejected when the resulting `used_storage_bytes` would exceed quota.
- Quota is enforced for multipart uploads, custom resumable upload completion, and Tus finalization.

## Upload Settings

Upload settings control global upload size policy.

Modes:

- `unlimited`: no global max file size.
- `custom`: requires `maxFileSizeBytes`.

Per-user quota is enforced independently from global max file size.

## Sharing Settings

Sharing settings control public share behavior.

Important settings:

- `sharing.enabled`: global public sharing switch.
- `sharing.public_preview_enabled`: allows public preview when shares also allow preview.
- `sharing.public_download_enabled`: allows public download when shares also allow download.
- `sharing.allow_folder_sharing`: policy flag for folder sharing.
- `sharing.allow_permanent_links`: policy flag for non-expiring shares.
- `sharing.require_password_mode`: `optional`, `always`, or `disabled`.
- `sharing.default_expiration_hours`: default expiration window for new shares, where `0` means no default expiration.

The backend validates sharing setting types and allowed values.

## Preview Settings

Preview settings control private and public preview behavior.

Important settings:

- `preview.enabled`: global preview switch.
- `preview.public_preview_enabled`: public preview switch.
- `preview.office_enabled`: office-to-PDF conversion switch.
- `preview.media_enabled`: audio/video preview and video thumbnail switch.
- `preview.image_thumbnails_enabled`: thumbnail generation switch.
- `preview.text_max_bytes`: maximum text preview bytes.
- `preview.csv_max_rows`: maximum CSV preview rows.
- `preview.office_conversion_timeout_seconds`: LibreOffice conversion timeout.
- `preview.max_auto_generate_size_bytes`: max size for automatic preview generation.

## Storage Administration

Storage admin pages and APIs support:

- Storage summary.
- Recalculate user storage usage from file records.
- Cleanup expired upload sessions.
- Cleanup preview cache.

Use storage recalculation after manual DB/storage repair, restore operations, or suspected usage drift.

## Audit Logs

Audit logs track security-sensitive and administrative events such as:

- Login success/failure.
- Upload completion.
- Share creation/access/password checks/downloads.
- Admin setting changes.
- User management actions.

Audit logs are stored in PostgreSQL and should also be shipped to external log retention in production.

## System Health

Admin system pages expose health and runtime information. The public operational endpoint is `/healthz`; metrics are available at `/metrics`.

Use these endpoints from external monitoring, not only from the admin UI.
