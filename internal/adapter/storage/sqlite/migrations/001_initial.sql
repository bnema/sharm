-- +goose Up
CREATE TABLE media (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    original_name TEXT NOT NULL,
    original_path TEXT NOT NULL,
    converted_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    codec TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    retention_days INTEGER NOT NULL,
    file_size INTEGER NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    thumb_path TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL,
    expires_at DATETIME NOT NULL
);

CREATE TABLE jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    media_id TEXT NOT NULL REFERENCES media(id),
    type TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    error_message TEXT NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at DATETIME,
    completed_at DATETIME
);

CREATE INDEX idx_jobs_status ON jobs(status);
CREATE INDEX idx_jobs_media_id ON jobs(media_id);
CREATE INDEX idx_media_expires ON media(expires_at);
CREATE INDEX idx_media_status ON media(status);

-- +goose Down
DROP TABLE jobs;
DROP TABLE media;
