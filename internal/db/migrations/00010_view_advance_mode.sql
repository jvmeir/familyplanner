-- +goose Up
-- advance_mode controls when a screen advances: "timer" (dwell seconds) or
-- "on_end" (wait for its end-capable widgets, e.g. a video, to finish).
ALTER TABLE views ADD COLUMN advance_mode TEXT NOT NULL DEFAULT 'timer';

-- +goose Down
ALTER TABLE views DROP COLUMN advance_mode;
