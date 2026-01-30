package domain

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewMedia(t *testing.T) {
	tests := []struct {
		name          string
		mediaType     MediaType
		originalName  string
		originalPath  string
		retentionDays int
	}{
		{
			name:          "valid video media",
			mediaType:     MediaTypeVideo,
			originalName:  "test.mp4",
			originalPath:  "/uploads/test.mp4",
			retentionDays: 7,
		},
		{
			name:          "valid audio media",
			mediaType:     MediaTypeAudio,
			originalName:  "song.mp3",
			originalPath:  "/uploads/song.mp3",
			retentionDays: 30,
		},
		{
			name:          "valid image media",
			mediaType:     MediaTypeImage,
			originalName:  "photo.jpg",
			originalPath:  "/uploads/photo.jpg",
			retentionDays: 14,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := NewMedia(tt.mediaType, tt.originalName, tt.originalPath, tt.retentionDays)

			assert.NotEmpty(t, media.ID, "ID should be generated")
			assert.Len(t, media.ID, 8, "ID should be 8 characters")
			assert.Equal(t, tt.mediaType, media.Type, "Type should match")
			assert.Equal(t, tt.originalName, media.OriginalName, "OriginalName should match")
			assert.Equal(t, tt.originalPath, media.OriginalPath, "OriginalPath should match")
			assert.Equal(t, MediaStatusConverting, media.Status, "Status should be converting")
			assert.Equal(t, tt.retentionDays, media.RetentionDays, "RetentionDays should match")

			expectedExpiry := media.CreatedAt.AddDate(0, 0, tt.retentionDays)
			assert.WithinDuration(t, expectedExpiry, media.ExpiresAt, time.Second, "ExpiresAt should be CreatedAt + retention days")
		})
	}
}

func TestMedia_IsExpired(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt time.Time
		want      bool
	}{
		{
			name:      "future expiration",
			expiresAt: time.Now().Add(24 * time.Hour),
			want:      false,
		},
		{
			name:      "past expiration",
			expiresAt: time.Now().Add(-24 * time.Hour),
			want:      true,
		},
		{
			name:      "exact current time",
			expiresAt: time.Now().Add(time.Millisecond),
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := &Media{
				ExpiresAt: tt.expiresAt,
			}
			got := media.IsExpired()
			assert.Equal(t, tt.want, got, "IsExpired should return expected value")
		})
	}
}

func TestMedia_MarkAsDone(t *testing.T) {
	media := NewMedia(MediaTypeVideo, "test.mp4", "/uploads/test.mp4", 7)

	convertedPath := "/converted/test.mp4"
	codec := CodecH264
	width := 1920
	height := 1080
	thumbPath := "/thumbs/test.jpg"
	fileSize := int64(1024000)

	media.MarkAsDone(convertedPath, codec, width, height, thumbPath, fileSize)

	assert.Equal(t, MediaStatusDone, media.Status, "Status should be done")
	assert.Equal(t, convertedPath, media.ConvertedPath, "ConvertedPath should match")
	assert.Equal(t, codec, media.Codec, "Codec should match")
	assert.Equal(t, width, media.Width, "Width should match")
	assert.Equal(t, height, media.Height, "Height should match")
	assert.Equal(t, thumbPath, media.ThumbPath, "ThumbPath should match")
	assert.Equal(t, fileSize, media.FileSize, "FileSize should match")
}

func TestMedia_MarkAsFailed(t *testing.T) {
	media := NewMedia(MediaTypeVideo, "test.mp4", "/uploads/test.mp4", 7)

	errMsg := "conversion failed: unsupported format"
	err := errors.New(errMsg)

	media.MarkAsFailed(err)

	assert.Equal(t, MediaStatusFailed, media.Status, "Status should be failed")
	assert.Equal(t, errMsg, media.ErrorMessage, "ErrorMessage should match")
}
