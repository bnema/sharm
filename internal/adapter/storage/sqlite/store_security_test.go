package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientUploadMigrationDeduplicatesExistingVariants(t *testing.T) {
	registerHook()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "migration.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(1)
	goose.SetBaseFS(migrations)
	require.NoError(t, goose.SetDialect("sqlite3"))
	require.NoError(t, goose.UpTo(db, "migrations", 7))
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO media (id, type, original_name, original_path, retention_days, created_at, expires_at)
		VALUES ('media-1', 'video', 'fixture.mp4', 'fixture.mp4', 1, ?, ?)`, now, now.Add(time.Hour))
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO media_variants (media_id, codec, status, created_at) VALUES
		('media-1', 'h264', 'pending', ?), ('media-1', 'h264', 'done', ?)`, now.Add(-time.Hour), now)
	require.NoError(t, err)
	_, err = db.Exec(`INSERT INTO jobs (media_id, type, codec, fps, status) VALUES
		('media-1', 'convert', 'h264', 30, 'pending'), ('media-1', 'convert', 'h264', 30, 'pending')`)
	require.NoError(t, err)

	require.NoError(t, goose.UpTo(db, "migrations", 8))

	var count int
	var status string
	require.NoError(t, db.QueryRow(`SELECT COUNT(*), MAX(status) FROM media_variants
		WHERE media_id = 'media-1' AND codec = 'h264'`).Scan(&count, &status))
	assert.Equal(t, 1, count)
	assert.Equal(t, "done", status)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM jobs
		WHERE media_id = 'media-1' AND type = 'convert' AND codec = 'h264'`).Scan(&count))
	assert.Equal(t, 1, count)
	_, err = db.Exec(`INSERT INTO media_variants (media_id, codec, is_primary) VALUES
		('media-1', 'av1', 1)`)
	assert.Error(t, err)

	require.NoError(t, goose.DownTo(db, "migrations", 7))
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM media_variants
		WHERE media_id = 'media-1' AND codec = 'h264'`).Scan(&count))
	assert.Equal(t, 2, count)
	require.NoError(t, db.QueryRow(`SELECT COUNT(*) FROM jobs
		WHERE media_id = 'media-1' AND type = 'convert' AND codec = 'h264'`).Scan(&count))
	assert.Equal(t, 2, count)
}

func TestStoreEnforcesSingleUserAndSessionVersion(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })

	require.NoError(t, store.CreateUser("admin", "hash"))
	assert.Error(t, store.CreateUser("second", "hash"))

	user, err := store.GetUser("admin")
	require.NoError(t, err)
	assert.Zero(t, user.SessionVersion)

	require.NoError(t, store.IncrementSessionVersion(user.ID))
	user, err = store.GetUser("admin")
	require.NoError(t, err)
	assert.Equal(t, int64(1), user.SessionVersion)
}

func TestUpdateVariantDonePromotesOnlyOnePrimary(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	media := domain.NewMedia(domain.MediaTypeVideo, "fixture.mp4", "opaque-path", 1)
	require.NoError(t, store.Save(media))
	h264 := &domain.Variant{MediaID: media.ID, Codec: domain.CodecH264}
	av1 := &domain.Variant{MediaID: media.ID, Codec: domain.CodecAV1}
	require.NoError(t, store.SaveVariant(h264))
	require.NoError(t, store.SaveVariant(av1))
	h264.Primary = true
	h264.Origin = domain.VariantOriginServer
	require.NoError(t, store.UpdateVariantDone(h264))
	av1.Primary = true
	av1.Origin = domain.VariantOriginServer

	require.NoError(t, store.UpdateVariantDone(av1))

	variants, err := store.ListVariantsByMedia(media.ID)
	require.NoError(t, err)
	primaries := make([]domain.Variant, 0, 1)
	for i := range variants {
		if variants[i].Primary {
			primaries = append(primaries, variants[i])
		}
	}
	require.Len(t, primaries, 1)
	assert.Equal(t, domain.CodecAV1, primaries[0].Codec)
}
