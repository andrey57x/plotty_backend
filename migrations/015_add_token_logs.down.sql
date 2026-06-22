ALTER TABLE ai_jobs DROP COLUMN IF EXISTS prompt_tokens;
ALTER TABLE ai_jobs DROP COLUMN IF EXISTS completion_tokens;
ALTER TABLE ai_jobs DROP COLUMN IF EXISTS total_tokens;