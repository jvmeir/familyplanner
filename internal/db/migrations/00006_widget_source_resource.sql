-- +goose Up
-- Per-(widget,source) chosen resource (e.g. which Bring list / calendar / folder),
-- so one data source can be reused by widgets that each show a different resource.
ALTER TABLE widget_sources ADD COLUMN resource TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE widget_sources DROP COLUMN resource;
