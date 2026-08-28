package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/adapter/storage/osfs"
	"github.com/bnema/sharm/internal/adapter/storage/sqlite"
	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uploadTestConverter struct {
	probe *domain.ProbeResult
}

func (uploadTestConverter) Convert(string, string, string) (string, string, error) {
	return "", "", errors.New("not used")
}

func (uploadTestConverter) ConvertCodec(string, string, string, domain.Codec, int) (string, error) {
	return "", errors.New("not used")
}

func (uploadTestConverter) Thumbnail(string, string) error { return errors.New("not used") }
func (c uploadTestConverter) ProbeContext(context.Context, string) (*domain.ProbeResult, error) {
	return c.probe, nil
}

type uploadTestJobQueue struct {
	enqueued int
	mediaID  string
	codec    domain.Codec
}

func (q *uploadTestJobQueue) Enqueue(mediaID string, _ domain.JobType, codec domain.Codec, _ int) (*domain.Job, error) {
	q.enqueued++
	q.mediaID = mediaID
	q.codec = codec
	return &domain.Job{MediaID: mediaID, Codec: codec}, nil
}
func (*uploadTestJobQueue) GetActive(string, domain.JobType, domain.Codec) (*domain.Job, error) {
	return nil, domain.ErrNotFound
}
func (*uploadTestJobQueue) Claim() (*domain.Job, error) { return nil, nil }
func (*uploadTestJobQueue) Complete(int64) error        { return nil }
func (*uploadTestJobQueue) Fail(int64, string) error    { return nil }
func (*uploadTestJobQueue) UpdateProgress(int64, int) error {
	return nil
}
func (*uploadTestJobQueue) Heartbeat(int64) error { return nil }
func (*uploadTestJobQueue) ResetStalled() error   { return nil }

func newUploadIntegrationService(t *testing.T, probe *domain.ProbeResult, jobs ...*uploadTestJobQueue) (*UploadService, *sqlite.Store) {
	t.Helper()
	store, err := sqlite.NewStore(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Close() })
	require.NoError(t, store.CreateUser("admin", "hash"))
	log := nopLogger{}
	dataDir := t.TempDir()
	config := UploadConfig{
		ChunkSize:       4,
		SessionTTL:      time.Hour,
		MaxAssetBytes:   1 << 20,
		MaxSessionBytes: 2 << 20,
	}
	queue := &uploadTestJobQueue{}
	if len(jobs) > 0 {
		queue = jobs[0]
	}
	return NewUploadService(store, store, uploadTestConverter{probe: probe}, osfs.NewUploadBlobs(dataDir), config, log, queue), store
}

func h264Probe() *domain.ProbeResult {
	return &domain.ProbeResult{
		Format: domain.ProbeFormat{FormatName: "mov,mp4,m4a,3gp,3g2,mj2", Duration: "1"},
		Streams: []domain.ProbeStream{
			{
				CodecType: "video", CodecName: "h264", Profile: "High", Level: 40,
				Width: 640, Height: 360, PixFmt: "yuv420p", AvgFrameRate: "30/1",
			},
			{CodecType: "audio", CodecName: "aac"},
		},
	}
}

func mp4Fixture(payload []byte) []byte {
	result := []byte{0, 0, 0, 12, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm', 0, 0, 0, 8, 'm', 'o', 'o', 'v'}
	mdat := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(mdat[:4], uint32(len(mdat)))
	copy(mdat[4:8], "mdat")
	copy(mdat[8:], payload)
	return append(result, mdat...)
}

func writeAllChunks(t *testing.T, service *UploadService, session *domain.UploadSession, data []byte) domain.UploadAsset {
	t.Helper()
	asset := session.Assets[0]
	for index := range asset.TotalChunks {
		start := index * int(asset.ChunkSize)
		end := min(start+int(asset.ChunkSize), len(data))
		_, err := service.WriteChunk(1, session.ID, asset.ID, index, "", bytes.NewReader(data[start:end]))
		require.NoError(t, err)
	}
	return asset
}

func uploadPrimary(t *testing.T, service *UploadService, session *domain.UploadSession, data []byte) *domain.UploadAsset {
	t.Helper()
	asset := writeAllChunks(t, service, session, data)
	result, err := service.FinalizeAsset(1, session.ID, asset.ID)
	require.NoError(t, err)
	return result.Asset
}

func TestPrimaryCompatibilityRejectsUnsupportedStreams(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*domain.ProbeResult)
	}{
		{name: "non AAC audio", mutate: func(probe *domain.ProbeResult) {
			probe.Streams[1].CodecName = "opus"
		}},
		{name: "extra subtitle stream", mutate: func(probe *domain.ProbeResult) {
			probe.Streams = append(probe.Streams, domain.ProbeStream{CodecType: "subtitle", CodecName: "mov_text"})
		}},
		{name: "unsupported pixel format", mutate: func(probe *domain.ProbeResult) {
			probe.Streams[0].PixFmt = "yuv444p"
		}},
		{name: "excessive frame rate", mutate: func(probe *domain.ProbeResult) {
			probe.Streams[0].AvgFrameRate = "120/1"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			probe := h264Probe()
			tt.mutate(probe)
			assert.False(t, isPrimaryCompatible(probe, uploadMP4MIME, true, []string{"mp4"}))
		})
	}
}

func TestUploadServicePublishesDirectPrimaryAndIsIdempotent(t *testing.T) {
	service, store := newUploadIntegrationService(t, h264Probe())
	data := mp4Fixture([]byte("primary"))
	session, err := service.CreateSession(CreateUploadInput{
		UserID:        1,
		Filename:      "clip.mp4",
		PrimarySize:   int64(len(data)),
		RetentionDays: 7,
	})
	require.NoError(t, err)
	asset := session.Assets[0]

	_, err = service.WriteChunk(1, session.ID, asset.ID, 0, "", bytes.NewReader(data[:4]))
	require.NoError(t, err)
	_, err = service.WriteChunk(1, session.ID, asset.ID, 0, "", bytes.NewReader(data[:4]))
	require.NoError(t, err)
	_, err = service.WriteChunk(1, session.ID, asset.ID, 0, "", bytes.NewReader([]byte("xxxx")))
	assert.ErrorIs(t, err, domain.ErrChunkConflict)

	published := uploadPrimary(t, service, session, data)
	assert.Equal(t, domain.AssetStatusAvailable, published.Status)
	media, err := store.Get(session.MediaID)
	require.NoError(t, err)
	assert.Equal(t, domain.MediaStatusDone, media.Status)
	assert.False(t, media.OriginalAvailable)
	variant := media.BestVariant()
	require.NotNil(t, variant)
	assert.True(t, variant.Primary)
	assert.Equal(t, domain.VariantOriginClient, variant.Origin)
	assert.Equal(t, int64(len(data)), variant.FileSize)

	result, err := service.FinalizeAsset(1, session.ID, asset.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.AssetStatusAvailable, result.Asset.Status)
}

func TestUploadServiceKeepsOriginalIndependently(t *testing.T) {
	service, store := newUploadIntegrationService(t, h264Probe())
	data := mp4Fixture(nil)
	session, err := service.CreateSession(CreateUploadInput{
		UserID:        1,
		Filename:      "clip.mp4",
		PrimarySize:   int64(len(data)),
		OriginalSize:  int64(len(data)),
		KeepOriginal:  true,
		RetentionDays: 7,
	})
	require.NoError(t, err)
	primary := uploadPrimary(t, service, session, data)
	assert.Equal(t, domain.AssetStatusAvailable, primary.Status)

	refreshed, err := service.GetSession(1, session.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.UploadSessionStatusActive, refreshed.Status)
	original := refreshed.Assets[1]
	for index := range original.TotalChunks {
		start := index * int(original.ChunkSize)
		end := min(start+int(original.ChunkSize), len(data))
		_, writeErr := service.WriteChunk(1, refreshed.ID, original.ID, index, "", bytes.NewReader(data[start:end]))
		require.NoError(t, writeErr)
	}
	_, err = service.FinalizeAsset(1, refreshed.ID, original.ID)
	require.NoError(t, err)

	media, err := store.Get(session.MediaID)
	require.NoError(t, err)
	assert.True(t, media.OriginalAvailable)
	assert.NotEmpty(t, media.OriginalPath)
	final, err := service.GetSession(1, session.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.UploadSessionStatusCompleted, final.Status)
}

func TestUploadServiceReservesQuotaAtomicallyAcrossServiceInstances(t *testing.T) {
	first, _ := newUploadIntegrationService(t, h264Probe())
	second := *first
	data := mp4Fixture(nil)
	first.maxReserved = int64(len(data))
	second.maxReserved = int64(len(data))
	services := []*UploadService{first, &second}
	errs := make(chan error, len(services))
	var wg sync.WaitGroup
	for i := range services {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := services[index].CreateSession(CreateUploadInput{
				UserID: 1, Filename: fmt.Sprintf("fixture-%d.mp4", index), PrimarySize: int64(len(data)),
			})
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)

	var successes, quotaFailures int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, domain.ErrQuotaExceeded):
			quotaFailures++
		default:
			t.Fatalf("unexpected session creation error: %v", err)
		}
	}
	assert.Equal(t, 1, successes)
	assert.Equal(t, 1, quotaFailures)
}

func TestUploadServiceRejectsFinalizationAfterSessionExpiry(t *testing.T) {
	service, store := newUploadIntegrationService(t, h264Probe())
	data := mp4Fixture(nil)
	session, err := service.CreateSession(CreateUploadInput{UserID: 1, Filename: "clip.mp4", PrimarySize: int64(len(data))})
	require.NoError(t, err)
	asset := writeAllChunks(t, service, session, data)
	service.now = func() time.Time { return session.ExpiresAt.Add(time.Second) }

	_, err = service.FinalizeAsset(1, session.ID, asset.ID)

	assert.ErrorIs(t, err, domain.ErrUploadExpired)
	stored, getErr := store.GetUploadAsset(asset.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.AssetStatusUploading, stored.Status)
}

func TestUploadServiceReleasesFinalizationAfterTransientProbeFailure(t *testing.T) {
	service, store := newUploadIntegrationService(t, h264Probe())
	data := mp4Fixture(nil)
	session, err := service.CreateSession(CreateUploadInput{UserID: 1, Filename: "clip.mp4", PrimarySize: int64(len(data))})
	require.NoError(t, err)
	asset := writeAllChunks(t, service, session, data)
	service.converter = failingUploadProbe{}

	_, err = service.FinalizeAsset(1, session.ID, asset.ID)

	require.Error(t, err)
	stored, getErr := store.GetUploadAsset(asset.ID)
	require.NoError(t, getErr)
	assert.Equal(t, domain.AssetStatusUploading, stored.Status)
}

type failingUploadProbe struct{}

func (failingUploadProbe) ProbeContext(context.Context, string) (*domain.ProbeResult, error) {
	return nil, errors.New("probe temporarily unavailable")
}

func TestUploadServiceFallsBackToServerConversion(t *testing.T) {
	jobs := &uploadTestJobQueue{}
	probe := &domain.ProbeResult{
		Format:  domain.ProbeFormat{FormatName: "matroska,webm", Duration: "1"},
		Streams: []domain.ProbeStream{{CodecType: "video", CodecName: "hevc", Width: 640, Height: 360}},
	}
	service, store := newUploadIntegrationService(t, probe, jobs)
	data := append([]byte{0x1a, 0x45, 0xdf, 0xa3}, []byte("webm-source")...)
	session, err := service.CreateSession(CreateUploadInput{UserID: 1, Filename: "source.webm", PrimarySize: int64(len(data))})
	require.NoError(t, err)
	asset := uploadPrimary(t, service, session, data)
	assert.Equal(t, domain.AssetStatusAvailable, asset.Status)
	assert.Equal(t, 1, jobs.enqueued)
	assert.Equal(t, domain.CodecH264, jobs.codec)

	media, err := store.Get(session.MediaID)
	require.NoError(t, err)
	assert.Equal(t, domain.MediaStatusProcessing, media.Status)
	assert.False(t, media.OriginalAvailable)
	assert.Empty(t, media.OriginalPath)
	transient, err := store.GetMediaAsset(session.MediaID, domain.AssetRoleSourceTransient)
	require.NoError(t, err)
	assert.Equal(t, domain.AssetStatusAvailable, transient.Status)
	variant, err := store.GetVariantByMediaAndCodec(session.MediaID, domain.CodecH264)
	require.NoError(t, err)
	assert.Equal(t, domain.VariantStatusPending, variant.Status)
}
