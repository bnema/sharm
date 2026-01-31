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
			assert.Equal(t, MediaStatusPending, media.Status, "Status should be converting")
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

func TestMedia_BestVariantForAccept(t *testing.T) {
	av1Done := Variant{Codec: CodecAV1, Status: VariantStatusDone, Path: "/v/av1.webm"}
	h264Done := Variant{Codec: CodecH264, Status: VariantStatusDone, Path: "/v/h264.mp4"}
	h264Pending := Variant{Codec: CodecH264, Status: VariantStatusPending, Path: "/v/h264.mp4"}

	tests := []struct {
		name     string
		variants []Variant
		accept   string
		wantPath string
		wantNil  bool
	}{
		{
			name:     "empty accept falls back to first done variant",
			variants: []Variant{h264Done, av1Done},
			accept:   "",
			wantPath: h264Done.Path,
		},
		{
			name:     "wildcard returns AV1 preferred",
			variants: []Variant{h264Done, av1Done},
			accept:   "*/*",
			wantPath: av1Done.Path,
		},
		{
			name:     "video/mp4 only returns H264",
			variants: []Variant{av1Done, h264Done},
			accept:   "video/mp4",
			wantPath: h264Done.Path,
		},
		{
			name:     "video/webm only returns AV1",
			variants: []Variant{av1Done, h264Done},
			accept:   "video/webm",
			wantPath: av1Done.Path,
		},
		{
			name:     "webm preferred over mp4 by q value",
			variants: []Variant{av1Done, h264Done},
			accept:   "video/webm, video/mp4;q=0.9",
			wantPath: av1Done.Path,
		},
		{
			name:     "mp4 preferred over webm by q value",
			variants: []Variant{av1Done, h264Done},
			accept:   "video/mp4, video/webm;q=0.5",
			wantPath: h264Done.Path,
		},
		{
			name:     "no matching variant returns nil",
			variants: []Variant{av1Done},
			accept:   "video/mp4",
			wantNil:  true,
		},
		{
			name:     "only one done variant returned regardless of accept",
			variants: []Variant{h264Done, h264Pending},
			accept:   "video/webm, video/mp4",
			wantPath: h264Done.Path,
		},
		{
			name:     "video wildcard matches all video types",
			variants: []Variant{av1Done, h264Done},
			accept:   "video/*",
			wantPath: av1Done.Path,
		},
		{
			name:     "no done variants returns nil",
			variants: []Variant{h264Pending},
			accept:   "video/mp4",
			wantNil:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			media := &Media{Variants: tt.variants}
			got := media.BestVariantForAccept(tt.accept)
			if tt.wantNil {
				assert.Nil(t, got)
			} else {
				assert.NotNil(t, got)
				assert.Equal(t, tt.wantPath, got.Path)
			}
		})
	}
}

func TestMedia_MarkAsFailed(t *testing.T) {
	media := NewMedia(MediaTypeVideo, "test.mp4", "/uploads/test.mp4", 7)

	errMsg := "conversion failed: unsupported format"
	err := errors.New(errMsg)

	media.MarkAsFailed(err)

	assert.Equal(t, MediaStatusFailed, media.Status, "Status should be failed")
	assert.Equal(t, errMsg, media.ErrorMessage, "ErrorMessage should match")
}
