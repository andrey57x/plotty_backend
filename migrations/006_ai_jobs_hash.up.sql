ALTER TABLE ai_jobs ADD COLUMN content_hash VARCHAR(64);
CREATE INDEX idx_ai_jobs_hash ON ai_jobs(chapter_id, type, content_hash);