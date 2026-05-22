CREATE UNIQUE INDEX IF NOT EXISTS uq_file_previews_file_type
  ON file_previews(file_id, preview_type);

CREATE INDEX IF NOT EXISTS idx_preview_jobs_status_created
  ON preview_jobs(status, created_at);

CREATE INDEX IF NOT EXISTS idx_preview_jobs_file_created
  ON preview_jobs(file_id, created_at DESC);
