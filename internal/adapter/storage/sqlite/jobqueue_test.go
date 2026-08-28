package sqlite

import (
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobQueueRejectsConflictingFPSForActiveJob(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	media := domain.NewMedia(domain.MediaTypeVideo, "fixture.mp4", "opaque-path", 1)
	require.NoError(t, store.Save(media))
	queue := NewJobQueue(store)
	_, err = queue.Enqueue(media.ID, domain.JobTypeConvert, domain.CodecH264, 30)
	require.NoError(t, err)

	_, err = queue.Enqueue(media.ID, domain.JobTypeConvert, domain.CodecH264, 60)

	assert.ErrorIs(t, err, domain.ErrJobConflict)
}

func TestJobQueueCompleteAndFailRequireRunningLease(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	media := domain.NewMedia(domain.MediaTypeVideo, "fixture.mp4", "opaque-path", 1)
	require.NoError(t, store.Save(media))
	queue := NewJobQueue(store)
	job, err := queue.Enqueue(media.ID, domain.JobTypeConvert, domain.CodecH264, 30)
	require.NoError(t, err)
	assert.ErrorIs(t, queue.Complete(job.ID), domain.ErrNotFound)
	assert.ErrorIs(t, queue.Fail(job.ID, "stale"), domain.ErrNotFound)
	claimed, err := queue.Claim()
	require.NoError(t, err)
	require.NotNil(t, claimed)
	require.NoError(t, queue.UpdateProgress(claimed.ID, -1))
	running, err := queue.GetActive(media.ID, domain.JobTypeConvert, domain.CodecH264)
	require.NoError(t, err)
	assert.Zero(t, running.Progress)
	require.NoError(t, queue.UpdateProgress(claimed.ID, 101))
	running, err = queue.GetActive(media.ID, domain.JobTypeConvert, domain.CodecH264)
	require.NoError(t, err)
	assert.Equal(t, 100, running.Progress)
	require.NoError(t, queue.Complete(claimed.ID))

	assert.ErrorIs(t, queue.Complete(claimed.ID), domain.ErrNotFound)
	assert.ErrorIs(t, queue.Fail(claimed.ID, "stale"), domain.ErrNotFound)
}

func TestJobQueueHeartbeatRejectsInactiveJob(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	media := domain.NewMedia(domain.MediaTypeVideo, "fixture.mp4", "opaque-path", 1)
	require.NoError(t, store.Save(media))
	queue := NewJobQueue(store)
	job, err := queue.Enqueue(media.ID, domain.JobTypeConvert, domain.CodecH264, 30)
	require.NoError(t, err)

	err = queue.Heartbeat(job.ID)

	assert.ErrorIs(t, err, domain.ErrNotFound)
}
