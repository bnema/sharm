package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkService_StoreChunk(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{})

	data := []byte("chunk data")
	err := svc.StoreChunk("upload-123", 0, data)
	require.NoError(t, err)

	chunkPath := filepath.Join(tmpDir, "sharm-chunks", "upload-123", "0")
	content, err := os.ReadFile(chunkPath)
	require.NoError(t, err)
	assert.Equal(t, data, content)
}

func TestChunkService_StoreChunk_InvalidUploadID(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{})

	err := svc.StoreChunk("", 0, []byte("data"))
	assert.Error(t, err)

	err = svc.StoreChunk("../escape", 0, []byte("data"))
	assert.Error(t, err)
}

func TestChunkService_Assemble(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{})

	require.NoError(t, svc.StoreChunk("upload-456", 0, []byte("aaa")))
	require.NoError(t, svc.StoreChunk("upload-456", 1, []byte("bbb")))
	require.NoError(t, svc.StoreChunk("upload-456", 2, []byte("ccc")))

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
	svc := NewChunkService(tmpDir, &nopLogger{})

	require.NoError(t, svc.StoreChunk("upload-789", 0, []byte("aaa")))

	_, err := svc.Assemble("upload-789", 2)
	assert.Error(t, err)
}

func TestChunkService_Cleanup(t *testing.T) {
	tmpDir := t.TempDir()
	svc := NewChunkService(tmpDir, &nopLogger{})

	require.NoError(t, svc.StoreChunk("upload-clean", 0, []byte("data")))
	chunkDir := filepath.Join(tmpDir, "sharm-chunks", "upload-clean")
	require.DirExists(t, chunkDir)

	svc.Cleanup("upload-clean")
	assert.NoDirExists(t, chunkDir)
}
