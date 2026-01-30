package service

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestMediaService_Upload_Success(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("test content")

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Return(nil).
		Times(2)

	mockConverter.EXPECT().Convert(mock.AnythingOfType("string"), mock.AnythingOfType("string"), mock.AnythingOfType("string")).
		Return("", "", errors.New("not implemented")).
		Once()

	result, err := service.Upload("test.mp4", tmpFile, 7)

	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, domain.MediaTypeVideo, result.Type)
	assert.Equal(t, "test.mp4", result.OriginalName)
	assert.Equal(t, domain.MediaStatusConverting, result.Status)
	assert.Equal(t, 7, result.RetentionDays)
	assert.WithinDuration(t, time.Now().AddDate(0, 0, 7), result.ExpiresAt, time.Second)

	uploadPath := filepath.Join(tempDir, "uploads", "test.mp4")
	_, err = os.Stat(uploadPath)
	assert.NoError(t, err, "file should exist at upload path")

	// Give time for the convert goroutine to fail gracefully
	time.Sleep(10 * time.Millisecond)
}

func TestMediaService_Upload_CreateDirectoryFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)

	service := NewMediaService(mockStore, mockConverter, "/invalid/path/that/cannot/be/created/\x00")

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	result, err := service.Upload("test.mp4", tmpFile, 7)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to create upload directory")
}

func TestMediaService_Upload_FileMoveFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	tmpFile.Close()
	os.Remove(tmpFile.Name())

	result, err := service.Upload("test.mp4", tmpFile, 7)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to save upload")
}

func TestMediaService_Upload_StoreSaveFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	tmpFile, err := os.CreateTemp("", "test_upload_*.mp4")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("test content")

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Return(errors.New("store save failed")).
		Once()

	result, err := service.Upload("test.mp4", tmpFile, 7)

	// Give time for the goroutine to fail gracefully
	time.Sleep(10 * time.Millisecond)

	assert.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "failed to save media metadata")

	uploadPath := filepath.Join(tempDir, "uploads", "test.mp4")
	_, err = os.Stat(uploadPath)
	assert.True(t, os.IsNotExist(err), "file should be cleaned up after store save fails")
}

func TestMediaService_convert_Success(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "test.mp4")
	err = os.WriteFile(originalFile, []byte("test content"), 0644)
	require.NoError(t, err)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", originalFile, 7)

	var savedMedia *domain.Media
	saveCalled := &sync.WaitGroup{}
	saveCalled.Add(1)

	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Run(func(m *domain.Media) {
			savedMedia = m
			saveCalled.Done()
		}).
		Return(nil).
		Once()

	convertedPath := filepath.Join(tempDir, "converted", media.ID+".mp4")
	mockConverter.EXPECT().Convert(originalFile, filepath.Join(tempDir, "converted"), media.ID).
		Run(func(inputPath string, outputDir string, id string) {
			err := os.MkdirAll(outputDir, 0755)
			require.NoError(t, err)
			err = os.WriteFile(convertedPath, []byte("converted content"), 0644)
			require.NoError(t, err)
		}).
		Return(convertedPath, "h264", nil).
		Once()

	mockConverter.EXPECT().Probe(convertedPath).
		Return(1920, 1080, nil).
		Once()

	thumbPath := filepath.Join(tempDir, "converted", media.ID+"_thumb.jpg")
	mockConverter.EXPECT().Thumbnail(convertedPath, thumbPath).
		Return(nil).
		Once()

	saveCalledWithTimeout := make(chan struct{})
	go func() {
		service.convert(media)
		close(saveCalledWithTimeout)
	}()

	select {
	case <-saveCalledWithTimeout:
	case <-time.After(5 * time.Second):
		t.Fatal("convert did not complete in time")
	}

	saveCalled.Wait()

	assert.NotNil(t, savedMedia)
	assert.Equal(t, domain.MediaStatusDone, savedMedia.Status)
	assert.Equal(t, convertedPath, savedMedia.ConvertedPath)
	assert.Equal(t, domain.CodecH264, savedMedia.Codec)
	assert.Equal(t, 1920, savedMedia.Width)
	assert.Equal(t, 1080, savedMedia.Height)
	assert.Equal(t, thumbPath, savedMedia.ThumbPath)
	assert.NotZero(t, savedMedia.FileSize)

	_, err = os.Stat(originalFile)
	assert.True(t, os.IsNotExist(err), "original file should be removed after successful conversion")
}

func TestMediaService_convert_CreateConvertedDirFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "test.mp4")
	err = os.WriteFile(originalFile, []byte("test content"), 0644)
	require.NoError(t, err)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", originalFile, 7)

	convertedDir := filepath.Join(tempDir, "converted")
	err = os.WriteFile(convertedDir, []byte("this is a file, not a directory"), 0644)
	require.NoError(t, err)

	var savedMedia *domain.Media
	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Run(func(m *domain.Media) {
			savedMedia = m
		}).
		Return(nil).
		Once()

	service.convert(media)

	assert.NotNil(t, savedMedia)
	assert.Equal(t, domain.MediaStatusFailed, savedMedia.Status)
	assert.Contains(t, savedMedia.ErrorMessage, "failed to create converted directory")
}

func TestMediaService_convert_ConvertFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "test.mp4")
	err = os.WriteFile(originalFile, []byte("test content"), 0644)
	require.NoError(t, err)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", originalFile, 7)

	var savedMedia *domain.Media
	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Run(func(m *domain.Media) {
			savedMedia = m
		}).
		Return(nil).
		Once()

	mockConverter.EXPECT().Convert(originalFile, filepath.Join(tempDir, "converted"), media.ID).
		Return("", "", errors.New("convert failed")).
		Once()

	service.convert(media)

	assert.NotNil(t, savedMedia)
	assert.Equal(t, domain.MediaStatusFailed, savedMedia.Status)
	assert.Contains(t, savedMedia.ErrorMessage, "convert failed")
}

func TestMediaService_convert_ProbeFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "test.mp4")
	err = os.WriteFile(originalFile, []byte("test content"), 0644)
	require.NoError(t, err)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", originalFile, 7)

	var savedMedia *domain.Media
	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Run(func(m *domain.Media) {
			savedMedia = m
		}).
		Return(nil).
		Once()

	convertedPath := filepath.Join(tempDir, "converted", media.ID+".mp4")
	mockConverter.EXPECT().Convert(originalFile, filepath.Join(tempDir, "converted"), media.ID).
		Return(convertedPath, "h264", nil).
		Once()

	mockConverter.EXPECT().Probe(convertedPath).
		Return(0, 0, errors.New("probe failed")).
		Once()

	service.convert(media)

	assert.NotNil(t, savedMedia)
	assert.Equal(t, domain.MediaStatusFailed, savedMedia.Status)
	assert.Contains(t, savedMedia.ErrorMessage, "failed to probe video")
}

func TestMediaService_convert_ThumbnailFails(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	uploadDir := filepath.Join(tempDir, "uploads")
	err := os.MkdirAll(uploadDir, 0755)
	require.NoError(t, err)

	originalFile := filepath.Join(uploadDir, "test.mp4")
	err = os.WriteFile(originalFile, []byte("test content"), 0644)
	require.NoError(t, err)

	media := domain.NewMedia(domain.MediaTypeVideo, "test.mp4", originalFile, 7)

	var savedMedia *domain.Media
	mockStore.EXPECT().Save(mock.AnythingOfType("*domain.Media")).
		Run(func(m *domain.Media) {
			savedMedia = m
		}).
		Return(nil).
		Once()

	convertedPath := filepath.Join(tempDir, "converted", media.ID+".mp4")
	mockConverter.EXPECT().Convert(originalFile, filepath.Join(tempDir, "converted"), media.ID).
		Return(convertedPath, "h264", nil).
		Once()

	mockConverter.EXPECT().Probe(convertedPath).
		Return(1920, 1080, nil).
		Once()

	thumbPath := filepath.Join(tempDir, "converted", media.ID+"_thumb.jpg")
	mockConverter.EXPECT().Thumbnail(convertedPath, thumbPath).
		Return(errors.New("thumbnail failed")).
		Once()

	service.convert(media)

	assert.NotNil(t, savedMedia)
	assert.Equal(t, domain.MediaStatusFailed, savedMedia.Status)
	assert.Contains(t, savedMedia.ErrorMessage, "failed to generate thumbnail")
}

func TestMediaService_Get_Success(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

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
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

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
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

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
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

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
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

	mockStore.EXPECT().ListExpired().
		Return([]*domain.Media{}, nil).
		Once()

	err := service.Cleanup()

	assert.NoError(t, err)
}

func TestMediaService_Cleanup_ContinuesOnFileDeletionErrors(t *testing.T) {
	mockStore := mocks.NewMediaStoreMock(t)
	mockConverter := mocks.NewMediaConverterMock(t)
	tempDir := t.TempDir()

	service := NewMediaService(mockStore, mockConverter, tempDir)

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
