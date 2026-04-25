ALTER TABLE users ADD COLUMN IF NOT EXISTS bio TEXT;

CREATE TABLE IF NOT EXISTS reader_story_shelf (
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    story_id UUID NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    shelf VARCHAR(32) NOT NULL CHECK (
        shelf IN ('reading', 'planned', 'read', 'dropped', 'favorite')
    ),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, story_id)
);

CREATE INDEX IF NOT EXISTS idx_reader_shelf_user ON reader_story_shelf (user_id);
CREATE INDEX IF NOT EXISTS idx_reader_shelf_user_shelf ON reader_story_shelf (user_id, shelf);

CREATE TABLE IF NOT EXISTS user_collections (
    id UUID PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    title VARCHAR(200) NOT NULL,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT user_collections_title_nonempty CHECK (length(trim(title)) > 0)
);

CREATE INDEX IF NOT EXISTS idx_user_collections_user ON user_collections (user_id);

CREATE TABLE IF NOT EXISTS user_collection_stories (
    collection_id UUID NOT NULL REFERENCES user_collections (id) ON DELETE CASCADE,
    story_id UUID NOT NULL REFERENCES stories (id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (collection_id, story_id)
);

CREATE INDEX IF NOT EXISTS idx_user_collection_stories_collection ON user_collection_stories (collection_id);
