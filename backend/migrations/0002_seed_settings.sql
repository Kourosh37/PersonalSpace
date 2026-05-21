INSERT INTO system_settings (id, key, value, value_type, description, is_public, created_at, updated_at)
VALUES
  (gen_random_uuid(), 'app.name', '"Space"'::jsonb, 'string', 'Application name', true, now(), now()),
  (gen_random_uuid(), 'app.public_base_url', '"http://localhost"'::jsonb, 'string', 'Public base url', true, now(), now()),
  (gen_random_uuid(), 'upload.max_file_size_mode', '"unlimited"'::jsonb, 'string', 'Upload max file size mode', false, now(), now()),
  (gen_random_uuid(), 'upload.max_file_size_bytes', 'null'::jsonb, 'number|null', 'Upload max file size in bytes', false, now(), now()),
  (gen_random_uuid(), 'sharing.enabled', 'true'::jsonb, 'bool', 'Public sharing enabled', false, now(), now()),
  (gen_random_uuid(), 'sharing.public_preview_enabled', 'true'::jsonb, 'bool', 'Public preview enabled', false, now(), now()),
  (gen_random_uuid(), 'sharing.public_download_enabled', 'true'::jsonb, 'bool', 'Public download enabled', false, now(), now()),
  (gen_random_uuid(), 'preview.enabled', 'true'::jsonb, 'bool', 'File previews enabled', false, now(), now())
ON CONFLICT (key) DO NOTHING;