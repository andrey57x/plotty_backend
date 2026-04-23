ALTER TABLE story_lorebooks ADD COLUMN IF NOT EXISTS embedding vector(384);
CREATE INDEX ON story_lorebooks USING hnsw (embedding vector_cosine_ops);