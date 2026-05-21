CREATE INDEX IF NOT EXISTS idx_files_owner_deleted ON files(owner_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_folders_owner_deleted ON folders(owner_id, deleted_at);
CREATE INDEX IF NOT EXISTS idx_share_links_owner ON share_links(owner_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_share_links_token_hash ON share_links(token_hash);