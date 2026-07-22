-- +goose Up
-- A kiosk device can run a second, independent playlist in the corner (PiP).
-- 0 = no PiP (corner hidden). pip_config_json holds corner/size/muted presentation.
ALTER TABLE kiosk_devices ADD COLUMN pip_playlist_id INTEGER NOT NULL DEFAULT 0;
ALTER TABLE kiosk_devices ADD COLUMN pip_config_json TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE kiosk_devices DROP COLUMN pip_playlist_id;
ALTER TABLE kiosk_devices DROP COLUMN pip_config_json;
