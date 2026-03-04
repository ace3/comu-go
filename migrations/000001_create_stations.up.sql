CREATE TABLE IF NOT EXISTS stations (
    uid       TEXT PRIMARY KEY,
    id        TEXT NOT NULL UNIQUE,
    name      TEXT NOT NULL,
    type      TEXT NOT NULL,
    metadata  JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
