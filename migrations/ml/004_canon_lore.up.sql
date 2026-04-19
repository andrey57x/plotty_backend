CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE canon_lorebooks (
    id UUID PRIMARY KEY,
    fandom_slug VARCHAR(255) NOT NULL,
    entity_name VARCHAR(255),
    fact_text TEXT NOT NULL,
    embedding vector(384)
);

CREATE INDEX ON canon_lorebooks USING hnsw (embedding vector_cosine_ops);