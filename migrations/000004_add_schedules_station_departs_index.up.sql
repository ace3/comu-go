CREATE INDEX IF NOT EXISTS idx_schedules_station_id_departs_at
ON schedules (station_id, departs_at);
