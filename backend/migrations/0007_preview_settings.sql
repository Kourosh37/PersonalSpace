INSERT INTO system_settings (id, key, value, value_type, description, is_public, created_at, updated_at)
VALUES
  (gen_random_uuid(), 'sharing.allow_folder_sharing', 'true'::jsonb, 'bool', 'Allow folder sharing', false, now(), now()),
  (gen_random_uuid(), 'sharing.allow_permanent_links', 'true'::jsonb, 'bool', 'Allow non-expiring share links', false, now(), now()),
  (gen_random_uuid(), 'sharing.require_password_mode', '"optional"'::jsonb, 'string', 'Require password for shares: optional|always|disabled', false, now(), now()),
  (gen_random_uuid(), 'sharing.default_expiration_hours', '168'::jsonb, 'number', 'Default share expiration window in hours; 0 means no default expiration', false, now(), now()),
  (gen_random_uuid(), 'preview.public_preview_enabled', 'true'::jsonb, 'bool', 'Public share preview enabled', false, now(), now()),
  (gen_random_uuid(), 'preview.office_enabled', 'false'::jsonb, 'bool', 'Office document preview enabled', false, now(), now()),
  (gen_random_uuid(), 'preview.media_enabled', 'true'::jsonb, 'bool', 'Video/audio preview enabled', false, now(), now()),
  (gen_random_uuid(), 'preview.image_thumbnails_enabled', 'true'::jsonb, 'bool', 'Image thumbnail generation enabled', false, now(), now()),
  (gen_random_uuid(), 'preview.text_max_bytes', '1048576'::jsonb, 'number', 'Max bytes for text preview', false, now(), now()),
  (gen_random_uuid(), 'preview.csv_max_rows', '500'::jsonb, 'number', 'Max CSV preview rows', false, now(), now()),
  (gen_random_uuid(), 'preview.office_conversion_timeout_seconds', '120'::jsonb, 'number', 'Office conversion timeout in seconds', false, now(), now()),
  (gen_random_uuid(), 'preview.max_auto_generate_size_bytes', '104857600'::jsonb, 'number', 'Max file size for auto preview generation', false, now(), now())
ON CONFLICT (key) DO NOTHING;
