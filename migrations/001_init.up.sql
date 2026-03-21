CREATE TABLE stories (
    id UUID PRIMARY KEY,
    slug VARCHAR(255) UNIQUE NOT NULL,
    title VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE tags (
    id UUID PRIMARY KEY,
    category VARCHAR(50) NOT NULL,
    slug VARCHAR(255) UNIQUE NOT NULL,
    name VARCHAR(255) NOT NULL
);

CREATE TABLE chapters (
    id UUID PRIMARY KEY,
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    title VARCHAR(500) NOT NULL,
    content TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE story_tags (
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    tag_id UUID NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (story_id, tag_id)
);

CREATE TABLE ai_jobs (
    id UUID PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    story_id UUID REFERENCES stories(id) ON DELETE SET NULL,
    chapter_id UUID REFERENCES chapters(id) ON DELETE SET NULL,
    input_payload JSONB NOT NULL DEFAULT '{}',
    result_payload JSONB,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE generated_images (
    id UUID PRIMARY KEY,
    job_id UUID NOT NULL REFERENCES ai_jobs(id) ON DELETE CASCADE,
    chapter_id UUID REFERENCES chapters(id) ON DELETE SET NULL,
    prompt TEXT NOT NULL,
    image_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX idx_stories_updated_at ON stories (updated_at DESC);
CREATE INDEX idx_chapters_story_created ON chapters (story_id, created_at);
CREATE INDEX idx_story_tags_tag_story ON story_tags (tag_id, story_id);
CREATE INDEX idx_tags_slug ON tags (slug);
CREATE INDEX idx_tags_category ON tags (category);

INSERT INTO tags (id, category, slug, name) VALUES
    ('10000001-0001-4001-8001-000000000001', 'directionality', 'harry-potter', 'Гарри Поттер'),
    ('10000001-0001-4001-8001-000000000002', 'directionality', 'witcher', 'Ведьмак'),
    ('10000001-0001-4001-8001-000000000003', 'directionality', 'lord-of-the-rings', 'Властелин колец'),
    ('10000001-0001-4001-8001-000000000004', 'directionality', 'naruto', 'Наруто'),
    ('10000001-0001-4001-8001-000000000005', 'directionality', 'marvel', 'Марвел'),
    ('10000001-0001-4001-8001-000000000006', 'directionality', 'dc', 'DC'),
    ('10000001-0001-4001-8001-000000000007', 'directionality', 'sherlock', 'Шерлок'),
    ('10000001-0001-4001-8001-000000000008', 'directionality', 'star-wars', 'Звёздные войны'),
    ('10000001-0001-4001-8001-000000000009', 'directionality', 'game-of-thrones', 'Игра престолов'),
    ('10000001-0001-4001-8001-00000000000a', 'directionality', 'attack-on-titan', 'Атака титанов'),
    ('10000001-0001-4001-8001-00000000000b', 'directionality', 'originals', 'Ориджиналы'),

    ('10000002-0002-4002-8002-000000000001', 'genre', 'drama', 'Драма'),
    ('10000002-0002-4002-8002-000000000002', 'genre', 'humor', 'Юмор'),
    ('10000002-0002-4002-8002-000000000003', 'genre', 'mystery', 'Мистика'),
    ('10000002-0002-4002-8002-000000000004', 'genre', 'slice-of-life', 'Повседневность'),
    ('10000002-0002-4002-8002-000000000005', 'genre', 'fantasy', 'Фэнтези'),
    ('10000002-0002-4002-8002-000000000006', 'genre', 'adventure', 'Приключения'),

    ('10000003-0003-4003-8003-000000000001', 'warning', 'character-death', 'Смерть персонажа'),
    ('10000003-0003-4003-8003-000000000002', 'warning', 'violence', 'Насилие'),
    ('10000003-0003-4003-8003-000000000003', 'warning', 'ooc', 'OOC'),
    ('10000003-0003-4003-8003-000000000004', 'warning', 'profanity', 'Нецензурная лексика'),

    ('10000004-0004-4004-8004-000000000001', 'rating', 'g', 'G'),
    ('10000004-0004-4004-8004-000000000002', 'rating', 'pg-13', 'PG-13'),
    ('10000004-0004-4004-8004-000000000003', 'rating', 'r', 'R'),
    ('10000004-0004-4004-8004-000000000004', 'rating', 'nc-17', 'NC-17'),
    ('10000004-0004-4004-8004-000000000005', 'rating', 'nc-21', 'NC-21'),

    ('10000005-0005-4005-8005-000000000001', 'size', 'drabble', 'Драббл'),
    ('10000005-0005-4005-8005-000000000002', 'size', 'mini', 'Мини'),
    ('10000005-0005-4005-8005-000000000003', 'size', 'midi', 'Миди'),
    ('10000005-0005-4005-8005-000000000004', 'size', 'maxi', 'Макси'),

    ('10000006-0006-4006-8006-000000000001', 'completion', 'completed', 'Завершён'),
    ('10000006-0006-4006-8006-000000000002', 'completion', 'in-progress', 'В процессе'),
    ('10000006-0006-4006-8006-000000000003', 'completion', 'frozen', 'Заморожен');
