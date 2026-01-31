package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMediaService_Upload_VideoNoCodecs(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name()) //nolint:errcheck
	_, _ = tmpFile.WriteString("test content")

	probeResult := &domain.ProbeResult{
		RawJSON: "{}",
	}
	mockConverter.EXPECT().Probe(mock.AnythingOfType("string")).
		Return(probeResult, nil).
		Once()

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Return(nil).
		Once()

	mockStore.EXPECT().UpdateDone(mock.AnythingOfType("*domain.Media")).
		Return(nil).
		Once()

	mockJobQueue.EXPECT().Enqueue(mock.AnythingOfType("string"), domain.JobTypeThumbnail, domain.Codec(""), 0).
		Return(&domain.Job{}, nil).
		Once()

	result, err := service.Upload("test.mp4", tmpFile, 7, domain.MediaTypeVideo, nil, 0)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.MediaTypeVideo, result.Type)
	assert.Equal(t, "test.mp4", result.OriginalName)
	assert.Equal(t, domain.MediaStatusDone, result.Status)
	assert.Equal(t, 7, result.RetentionDays)
	assert.WithinDuration(t, time.Now().AddDate(0, 0, 7), result.ExpiresAt, time.Second)

	_, err = os.Stat(result.OriginalPath)
	assert.NoError(t, err, "file should exist at upload path")
}

func TestMediaService_Upload_VideoWithCodecs(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name()) //nolint:errcheck
	_, _ = tmpFile.WriteString("test content")

	probeResult := &domain.ProbeResult{
		RawJSON: "{}",
	}
	mockConverter.EXPECT().Probe(mock.AnythingOfType("string")).
		Return(probeResult, nil).
		Once()

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Return(nil).
		Once()

	mockStore.EXPECT().SaveVariant(mock.AnythingOfType("*domain.Variant")).
		Return(nil).
		Times(2)

	mockJobQueue.EXPECT().Enqueue(mock.AnythingOfType("string"), domain.JobTypeConvert, domain.CodecAV1, 30).
		Return(&domain.Job{}, nil).
		Once()

	mockJobQueue.EXPECT().Enqueue(mock.AnythingOfType("string"), domain.JobTypeConvert, domain.CodecH264, 30).
		Return(&domain.Job{}, nil).
		Once()

	codecs := []domain.Codec{domain.CodecAV1, domain.CodecH264}
	result, err := service.Upload("test.mp4", tmpFile, 7, domain.MediaTypeVideo, codecs, 30)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.MediaTypeVideo, result.Type)
	assert.Equal(t, domain.MediaStatusPending, result.Status)
}

func TestMediaService_Upload_CreateDirectoryFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, "/invalid/path/that/cannot/be/created/\x00")

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name()) //nolint:errcheck

	result, err := service.Upload("test.mp4", tmpFile, 7, domain.MediaTypeVideo, nil, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create upload directory")
}

func TestMediaService_Upload_FileMoveFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	_ = tmpFile.Close()
	_ = os.Remove(tmpFile.Name())

	result, err := service.Upload("test.mp4", tmpFile, 7, domain.MediaTypeVideo, nil, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to save upload")
}

func TestMediaService_Upload_StoreSaveFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name()) //nolint:errcheck
	_, _ = tmpFile.WriteString("test content")

	probeResult := &domain.ProbeResult{
		RawJSON: "{}",
	}
	mockConverter.EXPECT().Probe(mock.AnythingOfType("string")).
		Return(probeResult, nil).
		Once()

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Return(errors.New("store save failed")).
		Once()

	result, err := service.Upload("test.mp4", tmpFile, 7, domain.MediaTypeVideo, nil, 0)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to save media metadata")

	uploadPath := filepath.Join(tempDir, "uploads", "test.mp4")
	_, err = os.Stat(uploadPath)
	assert.True(t, os.IsNotExist(err), "file should be cleaned up after store save fails")
}

func TestMediaService_Get_Success(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", 7)

	mockStore.EXPECT().Get("media-id").
		Return(media, nil).
		Once()

	result, err := service.Get("media-id")

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, media, result)
}

func TestMediaService_Get_NotFound(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	mockStore.EXPECT().Get("media-id").
		Return(nil, errors.New("not found")).
		Once()

	result, err := service.Get("media-id")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "not found")
}

func TestMediaService_Get_Expired(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", "/path/to/test.mp4", -1)

	mockStore.EXPECT().Get("media-id").
		Return(media, nil).
		Once()

	result, err := service.Get("media-id")

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.ErrorIs(t, err, domain.ErrExpired)
}

func TestMediaService_Cleanup_Success(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	convertedDir := filepath.Join(tempDir, "converted")

	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(convertedDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "original.mp4")
	err = os.WriteFile(originalFile, []byte("original"), 0644)
	require.NoError(t, err)

	convertedFile := filepath.Join(convertedDir, "converted.mp4")
	err = os.WriteFile(convertedFile, []byte("converted"), 0644)
	require.NoError(t, err)

	thumbFile := filepath.Join(convertedDir, "thumb.jpg")
	err = os.WriteFile(thumbFile, []byte("thumb"), 0644)
	require.NoError(t, err)

	media := &domain.Media{
		ID:            "expired-media",
		OriginalPath:  originalFile,
		ConvertedPath: convertedFile,
		ThumbPath:     thumbFile,
	}

	mockStore.EXPECT().ListExpired().
		Return([]*domain.Media{media}, nil).
		Once()

	mockStore.EXPECT().Delete("expired-media").
		Return(nil).
		Once()

	err = service.Cleanup()

	assert.NoError(t, err)

	_, err = os.Stat(originalFile)
	assert.True(t, os.IsNotExist(err), "original file should be deleted")

	_, err = os.Stat(convertedFile)
	assert.True(t, os.IsNotExist(err), "converted file should be deleted")

	_, err = os.Stat(thumbFile)
	assert.True(t, os.IsNotExist(err), "thumbnail file should be deleted")
}

func TestMediaService_Cleanup_NoExpiredMedia(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	mockStore.EXPECT().ListExpired().
		Return([]*domain.Media{}, nil).
		Once()

	err := service.Cleanup()

	assert.NoError(t, err)
}

func TestMediaService_Cleanup_ContinuesOnFileDeletionErrors(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	mockJobQueue := mocks.NewJobQueueMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, mockJobQueue, tempDir)

	media := &domain.Media{
		ID:            "expired-media",
		OriginalPath:  "/nonexistent/original.mp4",
		ConvertedPath: "/nonexistent/converted.mp4",
		ThumbPath:     "/nonexistent/thumb.jpg",
	}

	mockStore.EXPECT().ListExpired().
		Return([]*domain.Media{media}, nil).
		Once()

	mockStore.EXPECT().Delete("expired-media").
		Return(nil).
		Once()

	err := service.Cleanup()

	assert.NoError(t, err, "cleanup should succeed even if file deletion fails")
}
