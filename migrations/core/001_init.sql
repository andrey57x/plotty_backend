-- +goose Up

-- 1. ФАНДОМЫ
CREATE TABLE fandoms (
    id UUID PRIMARY KEY,
    name VARCHAR(100) UNIQUE NOT NULL,
    description TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- 2. ПОЛЬЗОВАТЕЛИ
CREATE TABLE users (
    id UUID PRIMARY KEY,
    username VARCHAR(50) UNIQUE NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    avatar_url TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- 3. ИСТОРИИ
CREATE TABLE stories (
    id UUID PRIMARY KEY,
    author_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    fandom_id UUID NOT NULL REFERENCES fandoms(id) ON DELETE RESTRICT,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    status VARCHAR(20) NOT NULL, -- 'draft', 'published', 'archived'
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL
);

-- 4. ГЛАВЫ
CREATE TABLE chapters (
    id UUID PRIMARY KEY,
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    chapter_number INT NOT NULL,
    title VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    status VARCHAR(20) NOT NULL, -- 'draft', 'published'
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL,
    UNIQUE(story_id, chapter_number)
);

-- 5. ЛАЙКИ НА ИСТОРИЮ
CREATE TABLE story_likes (
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    story_id UUID NOT NULL REFERENCES stories(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    PRIMARY KEY (user_id, story_id)
);

-- Индексы
CREATE INDEX idx_stories_fandom ON stories(fandom_id);
CREATE INDEX idx_stories_status ON stories(status);
CREATE INDEX idx_chapters_story_status ON chapters(story_id, status);

-- +goose Down
DROP TABLE IF EXISTS story_likes;
DROP TABLE IF EXISTS chapters;
DROP TABLE IF EXISTS stories;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS fandoms;