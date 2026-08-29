package sqlite

import (
	"testing"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadAssetGuardedTransitionsDistinguishMissingAndInvalidState(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.CreateUser("admin", "hash"))
	media := domain.NewMedia(domain.MediaTypeVideo, "fixture.mp4", "", 1)
	require.NoError(t, store.Save(media))
	now := time.Now()
	session := &domain.UploadSession{
		ID:            "session-1",
		MediaID:       media.ID,
		UserID:        1,
		Filename:      "fixture.mp4",
		ExpectedBytes: 4,
		ReservedBytes: 4,
		Status:        domain.UploadSessionStatusActive,
		ExpiresAt:     now.Add(time.Hour),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	asset := domain.UploadAsset{
		ID:           "asset-1",
		SessionID:    session.ID,
		MediaID:      media.ID,
		Role:         domain.AssetRolePrimaryH264,
		Filename:     "fixture.mp4",
		ExpectedSize: 4,
		ChunkSize:    4,
		TotalChunks:  1,
		Status:       domain.AssetStatusUploading,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	require.NoError(t, store.CreateUploadSession(session, []domain.UploadAsset{asset}, 4))

	err = store.CompleteUploadAsset(asset.ID, "path", "hash", 4, now)
	assert.ErrorIs(t, err, domain.ErrInvalidUpload)
	err = store.CompleteUploadAsset("missing", "path", "hash", 4, now)
	assert.ErrorIs(t, err, domain.ErrNotFound)
	require.NoError(t, store.FailUploadAsset(asset.ID, "failed", now))
	err = store.FailUploadAsset(asset.ID, "failed again", now)
	assert.ErrorIs(t, err, domain.ErrInvalidUpload)
	err = store.FailUploadAsset("missing", "failed", now)
	assert.ErrorIs(t, err, domain.ErrNotFound)
}
