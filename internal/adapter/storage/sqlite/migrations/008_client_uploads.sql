-- +goose Up

-- Enrich variants with the metadata required to validate and serve a primary
-- client-provided MP4 independently from the optional original asset.
ALTER TABLE media_variants ADD COLUMN container TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN video_codec TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN audio_codec TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN has_audio INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_variants ADD COLUMN profile TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN level TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN mime_type TEXT NOT NULL DEFAULT '';
ALTER TABLE media_variants ADD COLUMN origin TEXT NOT NULL DEFAULT 'server';
ALTER TABLE media_variants ADD COLUMN is_primary INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_variants ADD COLUMN progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE media_variants ADD COLUMN duration_seconds REAL NOT NULL DEFAULT 0;

UPDATE media_variants
SET container = 'mp4',
    video_codec = 'h264',
    mime_type = 'video/mp4',
    origin = 'server',
    is_primary = 1
WHERE codec = 'h264' AND status = 'done';

CREATE TABLE media_assets (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('original', 'source-transient')),
    filename TEXT NOT NULL,
    path TEXT NOT NULL,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'available',
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (media_id, role)
);

CREATE INDEX idx_media_assets_media_id ON media_assets(media_id);
CREATE INDEX idx_media_assets_status ON media_assets(status);

-- Preserve the source file of every existing media as an explicit original
-- capability. New sessions only create this row when the user opts in.
INSERT INTO media_assets (id, media_id, role, filename, path, size_bytes, status, created_at, updated_at)
SELECT 'legacy-' || id, id, 'original', original_name, original_path, 0, 'available', created_at, created_at
FROM media
WHERE original_path <> ''
  AND NOT EXISTS (
      SELECT 1 FROM media_assets assets
      WHERE assets.media_id = media.id AND assets.role = 'original'
  );

CREATE TABLE upload_sessions (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id),
    filename TEXT NOT NULL,
    retention_days INTEGER NOT NULL,
    keep_original INTEGER NOT NULL DEFAULT 0,
    expected_bytes INTEGER NOT NULL DEFAULT 0,
    reserved_bytes INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_upload_sessions_user_id ON upload_sessions(user_id);
CREATE INDEX idx_upload_sessions_expires_at ON upload_sessions(expires_at);
CREATE INDEX idx_upload_sessions_status ON upload_sessions(status);

CREATE TABLE upload_assets (
    id TEXT PRIMARY KEY,
    session_id TEXT NOT NULL REFERENCES upload_sessions(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('primary-h264', 'original', 'source-transient')),
    filename TEXT NOT NULL,
    expected_size INTEGER NOT NULL,
    chunk_size INTEGER NOT NULL,
    total_chunks INTEGER NOT NULL,
    received_bytes INTEGER NOT NULL DEFAULT 0,
    expected_sha256 TEXT NOT NULL DEFAULT '',
    sha256 TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'uploading',
    path TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    UNIQUE (session_id, role)
);

CREATE INDEX idx_upload_assets_session_id ON upload_assets(session_id);
CREATE INDEX idx_upload_assets_status ON upload_assets(status);

CREATE TABLE upload_chunks (
    asset_id TEXT NOT NULL REFERENCES upload_assets(id) ON DELETE CASCADE,
    chunk_index INTEGER NOT NULL,
    size_bytes INTEGER NOT NULL,
    sha256 TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (asset_id, chunk_index)
);

ALTER TABLE jobs ADD COLUMN progress INTEGER NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN lease_until DATETIME;
ALTER TABLE jobs ADD COLUMN heartbeat_at DATETIME;
ALTER TABLE jobs ADD COLUMN max_attempts INTEGER NOT NULL DEFAULT 3;

-- Existing databases may contain repeated variants for the same codec. Keep
-- rollback copies, then retain the newest completed row when possible before
-- enforcing idempotent creation.
CREATE TABLE migration_008_duplicate_variants AS
SELECT * FROM media_variants
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY media_id, codec
                   ORDER BY CASE WHEN status = 'done' THEN 0 ELSE 1 END,
                            created_at DESC,
                            id DESC
               ) AS duplicate_rank
        FROM media_variants
    )
    WHERE duplicate_rank > 1
);

DELETE FROM media_variants
WHERE id IN (
    SELECT id FROM (
        SELECT id,
               ROW_NUMBER() OVER (
                   PARTITION BY media_id, codec
                   ORDER BY CASE WHEN status = 'done' THEN 0 ELSE 1 END,
                            created_at DESC,
                            id DESC
               ) AS duplicate_rank
        FROM media_variants
    )
    WHERE duplicate_rank > 1
);

-- A media item has exactly one primary variant. Prefer a completed H.264 row
-- when old data contains multiple rows marked primary.
UPDATE media_variants
SET is_primary = 0
WHERE is_primary = 1
  AND id NOT IN (
      SELECT id FROM (
          SELECT id,
                 ROW_NUMBER() OVER (
                     PARTITION BY media_id
                     ORDER BY CASE WHEN codec = 'h264' AND status = 'done' THEN 0 ELSE 1 END,
                              created_at DESC,
                              id DESC
                 ) AS primary_rank
          FROM media_variants
          WHERE is_primary = 1
      )
      WHERE primary_rank = 1
  );

CREATE UNIQUE INDEX idx_media_variants_media_codec ON media_variants(media_id, codec);
CREATE UNIQUE INDEX idx_media_variants_one_primary ON media_variants(media_id) WHERE is_primary = 1;

-- Keep at most one recoverable job for the same media/variant. Preserve the
-- removed rows for rollback; completed and failed history remains untouched.
CREATE TABLE migration_008_duplicate_jobs AS
SELECT * FROM jobs
WHERE status IN ('pending', 'running')
  AND id NOT IN (
      SELECT MAX(id)
      FROM jobs
      WHERE status IN ('pending', 'running')
      GROUP BY media_id, type, codec
  );

DELETE FROM jobs
WHERE status IN ('pending', 'running')
  AND id NOT IN (
      SELECT MAX(id)
      FROM jobs
      WHERE status IN ('pending', 'running')
      GROUP BY media_id, type, codec
  );
CREATE UNIQUE INDEX idx_jobs_active_media_type_codec
    ON jobs(media_id, type, codec)
    WHERE status IN ('pending', 'running');

-- +goose Down
DROP INDEX IF EXISTS idx_jobs_active_media_type_codec;
DROP INDEX IF EXISTS idx_media_variants_one_primary;
DROP INDEX IF EXISTS idx_media_variants_media_codec;

INSERT INTO media_variants (
    id, media_id, codec, path, file_size, width, height, status, error_message,
    created_at, container, video_codec, audio_codec, has_audio, profile, level,
    mime_type, origin, is_primary, progress, duration_seconds
)
SELECT id, media_id, codec, path, file_size, width, height, status, error_message,
       created_at, container, video_codec, audio_codec, has_audio, profile, level,
       mime_type, origin, is_primary, progress, duration_seconds
FROM migration_008_duplicate_variants;

INSERT INTO jobs (
    id, media_id, type, status, error_message, attempts, created_at, started_at,
    completed_at, codec, fps, progress, lease_until, heartbeat_at, max_attempts
)
SELECT id, media_id, type, status, error_message, attempts, created_at, started_at,
       completed_at, codec, fps, progress, lease_until, heartbeat_at, max_attempts
FROM migration_008_duplicate_jobs;

DROP TABLE migration_008_duplicate_jobs;
DROP TABLE migration_008_duplicate_variants;
ALTER TABLE jobs DROP COLUMN max_attempts;
ALTER TABLE jobs DROP COLUMN heartbeat_at;
ALTER TABLE jobs DROP COLUMN lease_until;
ALTER TABLE jobs DROP COLUMN progress;
DROP TABLE IF EXISTS upload_chunks;
DROP TABLE IF EXISTS upload_assets;
DROP TABLE IF EXISTS upload_sessions;
DROP TABLE IF EXISTS media_assets;
ALTER TABLE media_variants DROP COLUMN duration_seconds;
ALTER TABLE media_variants DROP COLUMN progress;
ALTER TABLE media_variants DROP COLUMN is_primary;
ALTER TABLE media_variants DROP COLUMN origin;
ALTER TABLE media_variants DROP COLUMN mime_type;
ALTER TABLE media_variants DROP COLUMN level;
ALTER TABLE media_variants DROP COLUMN profile;
ALTER TABLE media_variants DROP COLUMN has_audio;
ALTER TABLE media_variants DROP COLUMN audio_codec;
ALTER TABLE media_variants DROP COLUMN video_codec;
ALTER TABLE media_variants DROP COLUMN container;
