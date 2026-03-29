package service

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bnema/sharm/internal/port"
)

// ChunkService manages temporary chunk storage and assembly for chunked uploads.
type ChunkService struct {
	baseDir string
	log     port.Logger
}

func NewChunkService(baseDir string, log port.Logger) *ChunkService {
	return &ChunkService{baseDir: baseDir, log: log}
}

// ValidateUploadID checks that uploadID is safe for use as a directory name.
func ValidateUploadID(uploadID string) bool {
	if uploadID == "" || len(uploadID) > 64 {
		return false
	}
	for _, c := range uploadID {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

func (s *ChunkService) chunkDir(uploadID string) string {
	return filepath.Join(s.baseDir, "sharm-chunks", uploadID)
}

// StoreChunk writes a single chunk to disk.
func (s *ChunkService) StoreChunk(uploadID string, index int, data []byte) error {
	if !ValidateUploadID(uploadID) {
		return fmt.Errorf("invalid upload ID")
	}

	dir := s.chunkDir(uploadID)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("create chunk dir: %w", err)
	}

	chunkPath := filepath.Join(dir, strconv.Itoa(index))
	if err := os.WriteFile(chunkPath, data, 0600); err != nil {
		return fmt.Errorf("write chunk %d: %w", index, err)
	}

	return nil
}

// Assemble concatenates all chunks into a single temporary file and returns it.
// The caller is responsible for closing and removing the returned file.
func (s *ChunkService) Assemble(uploadID string, totalChunks int) (*os.File, error) {
	if !ValidateUploadID(uploadID) {
		return nil, fmt.Errorf("invalid upload ID")
	}

	assembled, err := os.CreateTemp("", "upload-assembled-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create assembled file: %w", err)
	}

	dir := s.chunkDir(uploadID)
	for i := range totalChunks {
		chunkPath := filepath.Join(dir, strconv.Itoa(i))
		chunk, openErr := os.Open(chunkPath)
		if openErr != nil {
			_ = assembled.Close()
			_ = os.Remove(assembled.Name())
			return nil, fmt.Errorf("missing chunk %d: %w", i, openErr)
		}
		_, copyErr := io.Copy(assembled, chunk)
		_ = chunk.Close()
		if copyErr != nil {
			_ = assembled.Close()
			_ = os.Remove(assembled.Name())
			return nil, fmt.Errorf("copy chunk %d: %w", i, copyErr)
		}
	}

	if _, err := assembled.Seek(0, 0); err != nil {
		_ = assembled.Close()
		_ = os.Remove(assembled.Name())
		return nil, fmt.Errorf("seek assembled file: %w", err)
	}

	return assembled, nil
}

// Cleanup removes the chunk directory for an upload.
func (s *ChunkService) Cleanup(uploadID string) {
	dir := s.chunkDir(uploadID)
	if err := os.RemoveAll(dir); err != nil {
		s.log.Errorf("failed to cleanup chunk dir %s: %v", dir, err)
	}
}
