DROP INDEX IF EXISTS idx_stories_author;
ALTER TABLE stories DROP COLUMN IF EXISTS author_id;

DROP INDEX IF EXISTS idx_users_username;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
