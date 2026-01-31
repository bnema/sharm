-- +goose Up
ALTER TABLE media ADD COLUMN probe_json TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE media DROP COLUMN probe_json;
