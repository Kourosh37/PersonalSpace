ALTER TABLE files ALTER COLUMN folder_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_files_owner_folder_active
ON files(owner_id, folder_id)
WHERE deleted_at IS NULL;