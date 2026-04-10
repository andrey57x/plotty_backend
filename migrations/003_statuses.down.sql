DROP INDEX IF EXISTS idx_chapters_status;
ALTER TABLE chapters DROP COLUMN IF EXISTS status;

DROP INDEX IF EXISTS idx_stories_status;
ALTER TABLE stories DROP COLUMN IF EXISTS status;