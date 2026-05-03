ALTER TABLE chapters ADD COLUMN draft_title VARCHAR(500);
ALTER TABLE chapters ADD COLUMN draft_content TEXT;

-- Заполняем новые колонки текущими данными для существующих глав
UPDATE chapters SET draft_title = title, draft_content = content;

-- Теперь можно сделать их обязательными
ALTER TABLE chapters ALTER COLUMN draft_title SET NOT NULL;
ALTER TABLE chapters ALTER COLUMN draft_content SET NOT NULL;