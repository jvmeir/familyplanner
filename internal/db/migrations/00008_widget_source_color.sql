-- +goose Up
-- Per-(widget,source) colour, so a calendar widget can colour-code events by
-- which data source they came from (e.g. school = green, sport = orange).
ALTER TABLE widget_sources ADD COLUMN color TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE widget_sources DROP COLUMN color;
