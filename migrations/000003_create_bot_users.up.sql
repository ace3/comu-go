CREATE TABLE IF NOT EXISTS bot_users (
    telegram_id    BIGINT PRIMARY KEY,
    home_station   JSONB,
    away_station   JSONB,
    morning_time   VARCHAR(5),
    evening_time   VARCHAR(5),
    work_days      JSONB NOT NULL DEFAULT '["mon","tue","wed","thu","fri"]',
    notifications  BOOLEAN NOT NULL DEFAULT false,
    lang           VARCHAR(2) NOT NULL DEFAULT 'en',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS one_time_alerts (
    id             UUID PRIMARY KEY,
    telegram_id    BIGINT NOT NULL REFERENCES bot_users(telegram_id) ON DELETE CASCADE,
    origin         JSONB NOT NULL,
    dest           JSONB NOT NULL,
    scheduled_for  TIMESTAMPTZ NOT NULL,
    sent_at        TIMESTAMPTZ,
    success        BOOLEAN,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_one_time_alerts_telegram_id ON one_time_alerts (telegram_id);
CREATE INDEX IF NOT EXISTS idx_one_time_alerts_scheduled_for ON one_time_alerts (scheduled_for) WHERE sent_at IS NULL;
