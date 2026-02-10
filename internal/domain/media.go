package domain

import (
	"crypto/rand"
	"encoding/base32"
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"strings"
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
	MediaStatusPending    MediaStatus = "pending"
	MediaStatusProcessing MediaStatus = "processing"
	MediaStatusDone       MediaStatus = "done"
	MediaStatusFailed     MediaStatus = "failed"
)

type Codec string

const (
	CodecAV1  Codec = "av1"
	CodecH264 Codec = "h264"
	CodecOpus Codec = "opus"
)

type VariantStatus string

const (
	VariantStatusPending    VariantStatus = "pending"
	VariantStatusProcessing VariantStatus = "processing"
	VariantStatusDone       VariantStatus = "done"
	VariantStatusFailed     VariantStatus = "failed"
)

type Variant struct {
	ID           int64         `json:"id"`
	MediaID      string        `json:"media_id"`
	Codec        Codec         `json:"codec"`
	Path         string        `json:"path"`
	FileSize     int64         `json:"file_size"`
	Width        int           `json:"width"`
	Height       int           `json:"height"`
	Status       VariantStatus `json:"status"`
	ErrorMessage string        `json:"error_message"`
	CreatedAt    time.Time     `json:"created_at"`
}

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
	Variants      []Variant   `json:"variants"`
	ProbeJSON     string      `json:"probe_json"`
}

func NewMedia(mediaType MediaType, originalName, originalPath string, retentionDays int) *Media {
	id := generateID()

	return &Media{
		ID:            id,
		Type:          mediaType,
		OriginalName:  originalName,
		OriginalPath:  originalPath,
		Status:        MediaStatusPending,
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

// DaysRemaining returns the number of days until expiration (rounded up).
// Returns 0 if already expired.
func (m *Media) DaysRemaining() int {
	remaining := time.Until(m.ExpiresAt).Hours() / 24
	if remaining <= 0 {
		return 0
	}
	return int(math.Ceil(remaining))
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

// AllVariantsTerminal returns true if every variant has reached a terminal state (done or failed).
func (m *Media) AllVariantsTerminal() bool {
	for _, v := range m.Variants {
		if v.Status != VariantStatusDone && v.Status != VariantStatusFailed {
			return false
		}
	}
	return true
}

// HasDoneVariant returns true if at least one variant completed successfully.
func (m *Media) HasDoneVariant() bool {
	for _, v := range m.Variants {
		if v.Status == VariantStatusDone {
			return true
		}
	}
	return false
}

// BestVariant returns the first done variant, or nil if none.
func (m *Media) BestVariant() *Variant {
	for i := range m.Variants {
		if m.Variants[i].Status == VariantStatusDone {
			return &m.Variants[i]
		}
	}
	return nil
}

// codecMIME maps codecs to their MIME types.
var codecMIME = map[Codec]string{
	CodecAV1:  "video/webm",
	CodecH264: "video/mp4",
	CodecOpus: "audio/ogg",
}

// codecPriority defines tie-break order (lower = preferred).
var codecPriority = map[Codec]int{
	CodecAV1:  0,
	CodecH264: 1,
	CodecOpus: 2,
}

type acceptEntry struct {
	mime string
	q    float64
}

// parseAccept parses an HTTP Accept header value into a slice of entries.
func parseAccept(header string) []acceptEntry {
	var entries []acceptEntry
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		mime := part
		q := 1.0
		if idx := strings.Index(part, ";"); idx != -1 {
			mime = strings.TrimSpace(part[:idx])
			params := strings.TrimSpace(part[idx+1:])
			if strings.HasPrefix(params, "q=") {
				if v, err := strconv.ParseFloat(params[2:], 64); err == nil {
					q = v
				}
			}
		}
		entries = append(entries, acceptEntry{mime: mime, q: q})
	}
	return entries
}

// BestVariantForAccept returns the best done variant matching the given Accept
// header. Falls back to BestVariant() when Accept is empty or contains */*.
func (m *Media) BestVariantForAccept(accept string) *Variant {
	accept = strings.TrimSpace(accept)
	if accept == "" {
		return m.BestVariant()
	}

	entries := parseAccept(accept)

	type candidate struct {
		variant *Variant
		q       float64
		prio    int
	}
	var candidates []candidate

	for i := range m.Variants {
		v := &m.Variants[i]
		if v.Status != VariantStatusDone {
			continue
		}
		mime, ok := codecMIME[v.Codec]
		if !ok {
			continue
		}
		// Find the best matching accept entry for this variant
		bestQ := -1.0
		for _, e := range entries {
			if e.mime == "*/*" || e.mime == mime {
				if e.q > bestQ {
					bestQ = e.q
				}
			}
			// Handle type wildcards like video/* or audio/*
			if strings.HasSuffix(e.mime, "/*") {
				prefix := strings.TrimSuffix(e.mime, "*")
				if strings.HasPrefix(mime, prefix) && e.q > bestQ {
					bestQ = e.q
				}
			}
		}
		if bestQ >= 0 {
			candidates = append(candidates, candidate{
				variant: v,
				q:       bestQ,
				prio:    codecPriority[v.Codec],
			})
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.q > best.q || (c.q == best.q && c.prio < best.prio) {
			best = c
		}
	}
	return best.variant
}

// VariantByCodec returns the variant for a given codec, or nil.
func (m *Media) VariantByCodec(codec Codec) *Variant {
	for i := range m.Variants {
		if m.Variants[i].Codec == codec {
			return &m.Variants[i]
		}
	}
	return nil
}

func (m *Media) ParseProbe() (*ProbeResult, error) {
	if m.ProbeJSON == "" {
		return nil, nil
	}
	var result ProbeResult
	if err := json.Unmarshal([]byte(m.ProbeJSON), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
	".webp": true, ".svg": true, ".bmp": true, ".ico": true,
}

var audioExts = map[string]bool{
	".mp3": true, ".wav": true, ".ogg": true, ".flac": true,
	".aac": true, ".m4a": true, ".wma": true, ".opus": true,
}

func DetectMediaType(filename string) MediaType {
	ext := strings.ToLower(filepath.Ext(filename))
	if imageExts[ext] {
		return MediaTypeImage
	}
	if audioExts[ext] {
		return MediaTypeAudio
	}
	// Default to video for known video extensions or unknown types
	return MediaTypeVideo
}
