CREATE TABLE story_lorebooks (
    story_id UUID PRIMARY KEY,
    entities JSONB NOT NULL DEFAULT '{}', -- Храним JSON с персонажами, локациями и предметами
    summary TEXT,                         -- Сгенерированное summary без спойлеров
    last_processed_chapter_id UUID,       -- ID последней главы, которая обновила лор
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- Добавляем поле для структурированной ошибки в ai_tasks
ALTER TABLE ai_tasks ADD COLUMN IF NOT EXISTS error_details JSONB;