package service

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bnema/sharm/internal/adapter/storage/osfs"
	"github.com/bnema/sharm/internal/port"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
