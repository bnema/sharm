package service

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/sharm/internal/adapter/storage/osfs"
	"github.com/bnema/sharm/internal/port"
)

func testFS() port.FileSystem { return osfs.New() }

func TestChunkService_StoreChunk(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	err := svc.StoreChunk("upload-123", 0, bytes.NewReader([]byte("chunk data")))
	require.NoError(t, err)

	chunkPath := filepath.Join(tmpDir, "sharm-chunks", "upload-123", "0")
	content, err := os.ReadFile(chunkPath)
	require.NoError(t, err)
	assert.Equal(t, "chunk data", string(content))
}

func TestChunkService_StoreChunk_InvalidUploadID(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	err := svc.StoreChunk("", 0, strings.NewReader("data"))
	assert.Error(t, err)

	err = svc.StoreChunk("../escape", 0, strings.NewReader("data"))
	assert.Error(t, err)
}

func TestChunkService_Assemble(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	require.NoError(t, svc.StoreChunk("upload-456", 0, strings.NewReader("aaa")))
	require.NoError(t, svc.StoreChunk("upload-456", 1, strings.NewReader("bbb")))
	require.NoError(t, svc.StoreChunk("upload-456", 2, strings.NewReader("ccc")))

	assembled, err := svc.Assemble("upload-456", 3)
	require.NoError(t, err)
	defer func() {
		_ = assembled.Close()
		_ = os.Remove(assembled.Name())
	}()

	content, err := os.ReadFile(assembled.Name())
	require.NoError(t, err)
	assert.Equal(t, "aaabbbccc", string(content))
}

func TestChunkService_Assemble_MissingChunk(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	require.NoError(t, svc.StoreChunk("upload-789", 0, strings.NewReader("aaa")))

	_, err := svc.Assemble("upload-789", 2)
	assert.Error(t, err)
}

func TestChunkService_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	require.NoError(t, svc.StoreChunk("upload-clean", 0, strings.NewReader("data")))
	chunkDir := filepath.Join(tmpDir, "sharm-chunks", "upload-clean")
	require.DirExists(t, chunkDir)

	svc.Cleanup("upload-clean")
	assert.NoDirExists(t, chunkDir)
}

func TestChunkService_StoreChunk_AllowsRetriedChunkIndex(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	require.NoError(t, svc.StoreChunk("upload-retry", 3, strings.NewReader("first")))
	require.NoError(t, svc.StoreChunk("upload-retry", 3, strings.NewReader("second")))

	chunkPath := filepath.Join(tmpDir, "sharm-chunks", "upload-retry", "3")
	content, err := os.ReadFile(chunkPath)
	require.NoError(t, err)
	assert.Equal(t, "second", string(content))
}

func TestChunkService_StoreChunk_DoesNotCorruptExistingChunkOnWriteFailure(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{}, testFS())

	require.NoError(t, svc.StoreChunk("upload-retry", 3, strings.NewReader("stable")))

	err := svc.StoreChunk(
		"upload-retry",
		3,
		io.MultiReader(
			strings.NewReader("new"),
			iotest.ErrReader(errors.New("boom")),
		),
	)
	require.Error(t, err)

	chunkPath := filepath.Join(tmpDir, "sharm-chunks", "upload-retry", "3")
	content, readErr := os.ReadFile(chunkPath)
	require.NoError(t, readErr)
	assert.Equal(t, "stable", string(content))

	entries, listErr := os.ReadDir(filepath.Dir(chunkPath))
	require.NoError(t, listErr)
	assert.Len(t, entries, 1)
	assert.Equal(t, "3", entries[0].Name())
}
