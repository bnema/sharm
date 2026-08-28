package service

import (
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkerPoolVariantSourceUsesTransientAssetForH264Fallback(t *testing.T) {
	store := mocks.NewMediaStoreMock(t)
	store.EXPECT().GetMediaAsset("media-1", domain.AssetRoleSourceTransient).Return(&domain.MediaAsset{
		MediaID: "media-1",
		Role:    domain.AssetRoleSourceTransient,
		Path:    "opaque-source-path",
		Status:  domain.AssetStatusAvailable,
	}, nil)
	worker := &WorkerPool{store: store}

	path, transient, err := worker.variantSource(&domain.Media{ID: "media-1"}, domain.CodecH264)

	require.NoError(t, err)
	assert.Equal(t, "opaque-source-path", path)
	assert.True(t, transient)
}

func TestWorkerPoolVariantSourceRequiresOriginalForAV1(t *testing.T) {
	worker := &WorkerPool{store: mocks.NewMediaStoreMock(t)}

	path, transient, err := worker.variantSource(&domain.Media{ID: "media-1"}, domain.CodecAV1)

	assert.ErrorIs(t, err, domain.ErrUnsupportedMedia)
	assert.Empty(t, path)
	assert.False(t, transient)
}
