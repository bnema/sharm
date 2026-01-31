-- +goose Up

-- Variants table: stores per-format conversion info
CREATE TABLE media_variants (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    codec TEXT NOT NULL,
    path TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_variants_media_id ON media_variants(media_id);
CREATE INDEX idx_variants_status ON media_variants(status);

-- Add codec column to jobs for per-variant conversion
ALTER TABLE jobs ADD COLUMN codec TEXT NOT NULL DEFAULT '';

-- Migrate existing converted media into variants
INSERT INTO media_variants (media_id, codec, path, file_size, width, height, status, created_at)
SELECT id, codec, converted_path, file_size, width, height, 'done', created_at
FROM media
WHERE codec != '' AND converted_path != '' AND status = 'done';

-- +goose Down
DROP TABLE IF EXISTS media_variants;
