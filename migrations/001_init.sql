CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS authors (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS articles (
  id TEXT PRIMARY KEY,
  author_id TEXT NOT NULL REFERENCES authors(id),
  title TEXT NOT NULL,
  body TEXT NOT NULL CHECK (length(body) < 1000000),
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  search_vector TSVECTOR GENERATED ALWAYS AS (
    to_tsvector('simple', coalesce(title, '') || ' ' || coalesce(body, ''))
  ) STORED
) WITH (
  autovacuum_vacuum_insert_scale_factor = 0.01,
  autovacuum_analyze_scale_factor = 0.01
);

CREATE INDEX IF NOT EXISTS idx_articles_search
  ON articles USING GIN (search_vector);

CREATE INDEX IF NOT EXISTS idx_articles_created
  ON articles (created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_articles_author_created
  ON articles (author_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_authors_lower_name
  ON authors (lower(name));

-- Partial author-name search (LIKE '%...%'): trigram index, so a search
-- like 'Budi' also matches 'Budi Santoso' without a sequential scan.
CREATE INDEX IF NOT EXISTS idx_authors_name_trgm
  ON authors USING gin (lower(name) gin_trgm_ops);
