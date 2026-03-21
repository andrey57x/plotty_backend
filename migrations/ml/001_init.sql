-- +goose Up
CREATE TABLE ai_tasks (
    id UUID PRIMARY KEY,
    task_type VARCHAR(50) NOT NULL, -- spellcheck, image_gen
    payload TEXT NOT NULL,          -- исходный текст или промпт
    status VARCHAR(20) NOT NULL,    -- pending, processing, completed, failed
    result JSONB,                   -- результат работы ИИ
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- +goose Down
DROP TABLE IF EXISTS ai_tasks;