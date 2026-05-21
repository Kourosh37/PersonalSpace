CREATE INDEX IF NOT EXISTS idx_upload_sessions_owner_status ON upload_sessions(owner_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_upload_sessions_expires_at ON upload_sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_files_storage_key ON files(storage_key);