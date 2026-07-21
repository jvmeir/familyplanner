-- +goose Up
-- render_mode controls how a screen shows its widgets: "grid" (the split layout)
-- or "random_single" (one of the screen's widgets, picked at random each show).
ALTER TABLE views ADD COLUMN render_mode TEXT NOT NULL DEFAULT 'grid';

-- +goose Down
ALTER TABLE views DROP COLUMN render_mode;
