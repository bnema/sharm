package http

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestSendAllEvents_SkipsUnchangedFragments(t *testing.T) {
	h := NewSSEHandler(nil, nil, "example.com")
	media := &domain.Media{
		ID:            "abc12345",
		Type:          domain.MediaTypeVideo,
		OriginalName:  "demo.mp4",
		Status:        domain.MediaStatusProcessing,
		RetentionDays: 7,
		Variants: []domain.Variant{
			{Codec: domain.CodecAV1, Status: domain.VariantStatusProcessing},
		},
	}

	first := httptest.NewRecorder()
	state, err := h.sendAllEvents(first, media, nil)
	assert.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, 1, strings.Count(first.Body.String(), "event: status"))
	assert.Equal(t, 1, strings.Count(first.Body.String(), "event: row"))

	second := httptest.NewRecorder()
	state, err = h.sendAllEvents(second, media, state)
	assert.NoError(t, err)
	assert.NotNil(t, state)
	assert.Equal(t, "", second.Body.String())
}

func TestSendAllEvents_EmitsUpdatedFragments(t *testing.T) {
	h := NewSSEHandler(nil, nil, "example.com")
	processing := &domain.Media{
		ID:            "abc12345",
		Type:          domain.MediaTypeVideo,
		OriginalName:  "demo.mp4",
		Status:        domain.MediaStatusProcessing,
		RetentionDays: 7,
		Variants: []domain.Variant{
			{Codec: domain.CodecAV1, Status: domain.VariantStatusProcessing},
		},
	}
	done := &domain.Media{
		ID:            "abc12345",
		Type:          domain.MediaTypeVideo,
		OriginalName:  "demo.mp4",
		Status:        domain.MediaStatusDone,
		Codec:         domain.CodecAV1,
		ConvertedPath: "/tmp/demo.webm",
		RetentionDays: 7,
		FileSize:      1024,
		Variants: []domain.Variant{
			{Codec: domain.CodecAV1, Status: domain.VariantStatusDone, FileSize: 1024},
		},
	}

	first := httptest.NewRecorder()
	state, err := h.sendAllEvents(first, processing, nil)
	assert.NoError(t, err)

	second := httptest.NewRecorder()
	_, err = h.sendAllEvents(second, done, state)
	assert.NoError(t, err)
	assert.Equal(t, 1, strings.Count(second.Body.String(), "event: status"))
	assert.Equal(t, 1, strings.Count(second.Body.String(), "event: row"))
}
