CREATE TABLE chapter_lorebooks (
    chapter_id UUID PRIMARY KEY,
    story_id UUID NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    entities JSONB NOT NULL DEFAULT '{}',
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);
CREATE INDEX idx_chapter_lore_story ON chapter_lorebooks(story_id);