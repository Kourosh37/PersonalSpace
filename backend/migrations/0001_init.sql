CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE IF NOT EXISTS users (
  id uuid PRIMARY KEY,
  username text UNIQUE NOT NULL,
  password_hash text NOT NULL,
  role text NOT NULL DEFAULT 'user',
  is_active boolean NOT NULL DEFAULT true,
  storage_quota_bytes bigint NULL,
  used_storage_bytes bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS sessions (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash text UNIQUE NOT NULL,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  ip_address text NULL,
  user_agent text NULL
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS folders (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  parent_id uuid NULL REFERENCES folders(id) ON DELETE CASCADE,
  name text NOT NULL,
  path_cache text NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_folders_owner_parent_name_active ON folders(owner_id, parent_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS files (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  folder_id uuid NOT NULL REFERENCES folders(id) ON DELETE CASCADE,
  name text NOT NULL,
  original_name text NOT NULL,
  storage_key text NOT NULL,
  size_bytes bigint NOT NULL,
  mime_type text NULL,
  extension text NULL,
  checksum_sha256 text NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  deleted_at timestamptz NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_files_owner_folder_name_active ON files(owner_id, folder_id, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS upload_sessions (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  folder_id uuid NULL REFERENCES folders(id) ON DELETE SET NULL,
  file_id uuid NULL REFERENCES files(id) ON DELETE SET NULL,
  upload_protocol text NOT NULL,
  original_name text NOT NULL,
  target_name text NOT NULL,
  total_size_bytes bigint NULL,
  uploaded_bytes bigint NOT NULL DEFAULT 0,
  storage_key_temp text NOT NULL,
  status text NOT NULL,
  error_message text NULL,
  expires_at timestamptz NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS share_links (
  id uuid PRIMARY KEY,
  owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  target_type text NOT NULL,
  target_id uuid NOT NULL,
  token_hash text UNIQUE NOT NULL,
  token_preview text NULL,
  password_hash text NULL,
  expires_at timestamptz NULL,
  allow_preview boolean NOT NULL DEFAULT true,
  allow_download boolean NOT NULL DEFAULT true,
  allow_folder_browse boolean NOT NULL DEFAULT true,
  max_downloads int NULL,
  download_count int NOT NULL DEFAULT 0,
  is_revoked boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS preview_jobs (
  id uuid PRIMARY KEY,
  file_id uuid NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  job_type text NOT NULL,
  status text NOT NULL,
  output_storage_key text NULL,
  error_message text NULL,
  attempts int NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS file_previews (
  id uuid PRIMARY KEY,
  file_id uuid NOT NULL REFERENCES files(id) ON DELETE CASCADE,
  preview_type text NOT NULL,
  storage_key text NULL,
  mime_type text NULL,
  size_bytes bigint NULL,
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS system_settings (
  id uuid PRIMARY KEY,
  key text UNIQUE NOT NULL,
  value jsonb NOT NULL,
  value_type text NOT NULL,
  description text NULL,
  is_public boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS audit_logs (
  id uuid PRIMARY KEY,
  user_id uuid NULL REFERENCES users(id) ON DELETE SET NULL,
  action text NOT NULL,
  target_type text NULL,
  target_id uuid NULL,
  ip_address text NULL,
  user_agent text NULL,
  metadata jsonb NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(action);
CREATE INDEX IF NOT EXISTS idx_audit_logs_created_at ON audit_logs(created_at DESC);