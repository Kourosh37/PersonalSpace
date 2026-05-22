# API Contract (High-Level)

Base path: `/api`

## Auth

- `POST /auth/login` -> `{ user }` + session cookie
- `POST /auth/logout` -> `{ ok }`
- `GET /auth/me` -> `{ user }`
- `POST /auth/change-password` -> `{ ok }`

Errors: `400` invalid payload, `401` invalid/auth required, `403` forbidden.

## Uploads

### Multipart
- `POST /files/upload?folderId=<uuid?>`
- Multipart field: `file` (repeatable)
- Response: `{ results: [{ id?, name?, originalName, sizeBytes?, error? }] }`

### Custom resumable
- `POST /uploads/init`
- `PATCH /uploads/{id}/chunk` (uses `Upload-Offset`)
- `GET /uploads/{id}/status`
- `POST /uploads/{id}/complete`
- `DELETE /uploads/{id}/cancel`

### Tus resumable
- `POST /uploads/tus`
- `HEAD /uploads/tus/{id}`
- `PATCH /uploads/tus/{id}`
- `DELETE /uploads/tus/{id}`

Tus headers: `Tus-Resumable`, `Upload-Length`, `Upload-Offset`, `Upload-Metadata`.

## Files and Preview

- `GET /files/{id}`
- `GET /files/{id}/download`
- `GET /files/{id}/preview`
- `GET /files/{id}/preview-info`
- `GET /files/{id}/preview-content`

Supports byte ranges on stream endpoints (`Range`, `206`, `Content-Range`).

## Shares

Private management:
- `POST /shares`
- `GET /shares`
- `GET /shares/{id}`
- `PATCH /shares/{id}`
- `DELETE /shares/{id}`
- `POST /shares/{id}/revoke`

Public access:
- `GET /public/shares/{token}`
- `POST /public/shares/{token}/password`
- `GET /public/shares/{token}/items`
- `GET /public/shares/{token}/files/{fileId}/preview-info`
- `GET /public/shares/{token}/files/{fileId}/preview-content`
- `GET /public/shares/{token}/files/{fileId}/preview`
- `GET /public/shares/{token}/files/{fileId}/download`
- `GET /public/shares/{token}/folders/{folderId}/download-zip`

## Admin

- `GET/PATCH /admin/settings`
- `GET/PATCH /admin/settings/upload`
- `GET/PATCH /admin/settings/sharing`
- `GET/PATCH /admin/settings/preview`
- `GET /admin/storage/summary`
- `POST /admin/storage/recalculate`
- `GET /admin/audit-logs`
- `GET /admin/system/health`
- `GET /admin/system/info`

## Error model

Most endpoints return:

```json
{ "error": "message" }
```

Validation failures: `400`; unauthorized: `401`; forbidden/policy: `403`; not found: `404`; conflicts/range: `409/416`; server errors: `500`.
