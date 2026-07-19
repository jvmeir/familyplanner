-- +goose Up
-- Health/telemetry for data sources so the kiosk can surface inactive auth and
-- failed syncs without calling any external API. Written by the broker.
--   access_expiry : RFC3339 access-token expiry (OAuth sources), '' if unknown
--   last_sync     : datetime of the last successful use, '' if never
--   last_error    : most recent error message, '' if healthy
--   health        : '' (unknown) | ok | expired | reconnect | error
ALTER TABLE data_sources ADD COLUMN access_expiry TEXT NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN last_sync TEXT NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN last_error TEXT NOT NULL DEFAULT '';
ALTER TABLE data_sources ADD COLUMN health TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE data_sources DROP COLUMN access_expiry;
ALTER TABLE data_sources DROP COLUMN last_sync;
ALTER TABLE data_sources DROP COLUMN last_error;
ALTER TABLE data_sources DROP COLUMN health;
