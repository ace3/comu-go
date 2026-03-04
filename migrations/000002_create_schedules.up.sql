CREATE TABLE IF NOT EXISTS schedules (
    id             TEXT PRIMARY KEY,
    train_id       TEXT NOT NULL,
    line           TEXT NOT NULL,
    route          TEXT NOT NULL,
    origin_id      TEXT NOT NULL,
    destination_id TEXT NOT NULL,
    station_id     TEXT NOT NULL,
    departs_at     TIMESTAMPTZ NOT NULL,
    arrives_at     TIMESTAMPTZ NOT NULL,
    metadata       JSONB,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_schedules_train_id ON schedules (train_id);
CREATE INDEX IF NOT EXISTS idx_schedules_station_id ON schedules (station_id);
