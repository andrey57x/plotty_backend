DROP INDEX IF EXISTS idx_ai_jobs_hash;
ALTER TABLE ai_jobs DROP COLUMN IF EXISTS content_hash;