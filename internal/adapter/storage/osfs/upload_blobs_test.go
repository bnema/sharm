package osfs

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mp4Boxes(order ...string) []byte {
	var result []byte
	for _, boxType := range order {
		size := 8
		if boxType == "ftyp" {
			size = 12
		}
		box := make([]byte, size)
		binary.BigEndian.PutUint32(box[:4], uint32(size))
		copy(box[4:8], boxType)
		if boxType == "ftyp" {
			copy(box[8:12], "isom")
		}
		result = append(result, box...)
	}
	return result
}

func TestUploadBlobsStageDetectsFastStartMP4(t *testing.T) {
	store := NewUploadBlobs(t.TempDir())
	data := mp4Boxes("ftyp", "moov", "mdat")
	written, digest, err := store.WriteChunk("session-1", "asset-1", 0, int64(len(data)), "", bytes.NewReader(data))
	require.NoError(t, err)

	staged, err := store.Stage("session-1", "asset-1", "media-1", []domain.UploadChunk{{Index: 0, Size: written, SHA256: digest}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = store.Discard(staged.Path) })
	assert.Equal(t, "video/mp4", staged.MIME)
	assert.True(t, staged.FastStart)
	assert.Equal(t, digest, staged.SHA256)
}

func TestUploadBlobsStageRejectsMalformedMP4Boxes(t *testing.T) {
	store := NewUploadBlobs(t.TempDir())
	data := []byte{0, 0, 0, 4, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'}
	written, digest, err := store.WriteChunk("session-1", "asset-1", 0, int64(len(data)), "", bytes.NewReader(data))
	require.NoError(t, err)

	_, err = store.Stage("session-1", "asset-1", "media-1", []domain.UploadChunk{{Index: 0, Size: written, SHA256: digest}})

	assert.ErrorIs(t, err, domain.ErrInvalidUpload)
}

func TestUploadBlobsRemoveAssetDeletesCompletedChunkTree(t *testing.T) {
	store := NewUploadBlobs(t.TempDir())
	data := []byte("chunk")
	written, digest, err := store.WriteChunk("session-1", "asset-1", 0, int64(len(data)), "", bytes.NewReader(data))
	require.NoError(t, err)

	require.NoError(t, store.RemoveAsset("session-1", "asset-1"))
	_, err = store.Stage("session-1", "asset-1", "media-1", []domain.UploadChunk{{Index: 0, Size: written, SHA256: digest}})
	assert.Error(t, err)
}

func TestUploadBlobsWriteChunkRejectsConflictingExistingChunk(t *testing.T) {
	store := NewUploadBlobs(t.TempDir())
	original := []byte("first")
	_, originalDigest, err := store.WriteChunk(
		"session-1",
		"asset-1",
		0,
		int64(len(original)),
		"",
		bytes.NewReader(original),
	)
	require.NoError(t, err)

	_, _, err = store.WriteChunk(
		"session-1",
		"asset-1",
		0,
		int64(len(original)),
		"",
		bytes.NewReader([]byte("other")),
	)

	assert.ErrorIs(t, err, domain.ErrChunkConflict)
	_, digest, err := store.WriteChunk(
		"session-1",
		"asset-1",
		0,
		int64(len(original)),
		"",
		bytes.NewReader(original),
	)
	require.NoError(t, err)
	assert.Equal(t, originalDigest, digest)
}

func TestUploadBlobsWriteChunkRejectsDeclaredHashMismatchWithoutPublishing(t *testing.T) {
	dataDir := t.TempDir()
	store := NewUploadBlobs(dataDir)
	data := []byte("chunk")

	_, _, err := store.WriteChunk("session-1", "asset-1", 0, int64(len(data)), "not-the-hash", bytes.NewReader(data))
	require.ErrorIs(t, err, domain.ErrInvalidUpload)
	chunkPath := filepath.Join(dataDir, "uploads", "session-1", "asset-1", "00000000.part")
	_, statErr := os.Stat(chunkPath)
	assert.ErrorIs(t, statErr, os.ErrNotExist)

	_, _, err = store.WriteChunk("session-1", "asset-1", 0, int64(len(data)), "", bytes.NewReader(data))
	require.NoError(t, err)
}
