package domain

import (
	"crypto/rand"
	"encoding/base32"
	"time"
)

type MediaType string

const (
	MediaTypeVideo MediaType = "video"
	MediaTypeAudio MediaType = "audio"
	MediaTypeImage MediaType = "image"
)

type MediaStatus string

const (
	MediaStatusConverting MediaStatus = "converting"
	MediaStatusDone       MediaStatus = "done"
	MediaStatusFailed     MediaStatus = "failed"
)

type Codec string

const (
	CodecAV1  Codec = "av1"
	CodecH264 Codec = "h264"
)

type Media struct {
	ID            string      `json:"id"`
	Type          MediaType   `json:"type"`
	OriginalName  string      `json:"original_name"`
	OriginalPath  string      `json:"original_path"`
	ConvertedPath string      `json:"converted_path"`
	Status        MediaStatus `json:"status"`
	Codec         Codec       `json:"codec"`
	ErrorMessage  string      `json:"error_message"`
	RetentionDays int         `json:"retention_days"`
	FileSize      int64       `json:"file_size"`
	Width         int         `json:"width"`
	Height        int         `json:"height"`
	ThumbPath     string      `json:"thumb_path"`
	CreatedAt     time.Time   `json:"created_at"`
	ExpiresAt     time.Time   `json:"expires_at"`
}

func NewMedia(mediaType MediaType, originalName, originalPath string, retentionDays int) *Media {
	id := generateID()

	return &Media{
		ID:            id,
		Type:          mediaType,
		OriginalName:  originalName,
		OriginalPath:  originalPath,
		Status:        MediaStatusConverting,
		RetentionDays: retentionDays,
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().AddDate(0, 0, retentionDays),
	}
}

func generateID() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base32.StdEncoding.EncodeToString(b)[:8]
}

func (m *Media) IsExpired() bool {
	return time.Now().After(m.ExpiresAt)
}

func (m *Media) MarkAsDone(convertedPath string, codec Codec, width, height int, thumbPath string, fileSize int64) {
	m.Status = MediaStatusDone
	m.ConvertedPath = convertedPath
	m.Codec = codec
	m.Width = width
	m.Height = height
	m.ThumbPath = thumbPath
	m.FileSize = fileSize
}

func (m *Media) MarkAsFailed(err error) {
	m.Status = MediaStatusFailed
	m.ErrorMessage = err.Error()
}
