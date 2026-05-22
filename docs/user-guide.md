# User Guide

## Login

Open the Space URL configured by the administrator and sign in with your username and password.

After login, Space opens the dashboard. If your session expires or is invalidated, you are redirected to the login page.

## Browsing Files

The dashboard shows folders and files in the current location.

Common actions:

- Click a folder name to enter it.
- Use breadcrumbs to return to a parent folder or root.
- Use search to filter the current folder.
- Use sort controls to change ordering.

## Creating Folders

Enter a folder name in the dashboard and select `Create Folder`.

Folder names are normalized by the server. Slash characters are stripped to prevent path traversal and accidental nested path creation.

## Uploading Files

Use `Upload Files` for normal browser file uploads.

Expected behavior:

- If upload succeeds, the file appears in the current folder.
- If the file exceeds the configured upload size limit, upload is rejected.
- If the user quota is exceeded, upload is rejected.
- If an item with the same name exists in the folder, upload is rejected for that file.

## Resumable Uploads

Use the Tus upload panel for large files or unreliable networks.

Controls:

- Add files through the Uppy panel.
- Start upload from the panel.
- Pause, resume, or cancel the queue.
- Observe progress, speed, ETA, retry count, and status.

Resume behavior depends on browser support, Tus fingerprint persistence, and whether the same file is selected again.

## Uploading Folders

Select `Add Folder` in the Tus upload section.

Supported browsers expose directory selection APIs. Chromium-based browsers support this flow most consistently.

Nested folder paths are encoded into filenames in the current destination folder. This avoids storing an incomplete folder tree model from browser-only relative paths.

## Previewing Files

For supported files, select preview actions from the dashboard.

Supported preview examples:

- Images open as streams.
- PDFs open as streams.
- Audio/video open as media streams.
- Text/code files open as bounded text content.
- CSV files open as bounded tabular content.
- Office files require a generated PDF preview job.

If preview generation is needed, use the preview diagnostics controls where available.

## Downloading Files

Space supports:

- In-app download with progress tracking.
- Browser-native download.
- HTTP range streaming for compatible clients.
- Folder ZIP download.

## Sharing Files And Folders

Use `Share` on a file or folder to create a public share link.

Depending on administrator policy, shares may support:

- Preview.
- Download.
- Folder browsing.
- Password protection.
- Expiration.
- Download limits.

If a share stops working, it may be expired, revoked, over its download limit, or blocked by a global admin setting.

## Changing Password

Use `Change Password` in the dashboard. After a successful password change, you must sign in again.
