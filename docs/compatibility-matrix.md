# Compatibility Matrix

## Platform

- Docker Engine: validated on `24.x+`
- Docker Compose plugin: validated on `2.24+`

## Browsers

- Chromium-based (Chrome/Edge): full dashboard flow including folder upload UX.
- Firefox: core upload/download/share flows supported; folder upload UX may vary.
- Safari: core flows supported; resumable behavior can vary with tab suspension/background limits.

## Resume behavior constraints

- Tus/custom resumable uploads rely on browser/network continuity.
- Suspended/terminated tabs can interrupt active streams.
- Large uploads are bounded by admin file-size settings and user quota.

## Known constraints

- Public share access follows global admin sharing/preview toggles.
- Preview generation depends on worker availability and toolchain (`LibreOffice`, `ffmpeg`).
