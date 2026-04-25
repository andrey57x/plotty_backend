DROP INDEX IF EXISTS idx_user_collection_stories_collection;
DROP TABLE IF EXISTS user_collection_stories;

DROP INDEX IF EXISTS idx_user_collections_user_public;
DROP INDEX IF EXISTS idx_user_collections_user;
DROP TABLE IF EXISTS user_collections;

DROP INDEX IF EXISTS idx_reader_shelf_user_shelf;
DROP INDEX IF EXISTS idx_reader_shelf_user;
DROP TABLE IF EXISTS reader_story_shelf;

ALTER TABLE users DROP COLUMN IF EXISTS bio;
