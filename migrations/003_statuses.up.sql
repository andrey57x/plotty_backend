-- Добавляем статус для историй (по умолчанию черновик)
ALTER TABLE stories ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'draft';
CREATE INDEX IF NOT EXISTS idx_stories_status ON stories(status);

-- Добавляем статус для глав
ALTER TABLE chapters ADD COLUMN IF NOT EXISTS status VARCHAR(50) NOT NULL DEFAULT 'draft';
CREATE INDEX IF NOT EXISTS idx_chapters_status ON chapters(status);