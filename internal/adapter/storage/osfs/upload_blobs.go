package osfs

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

const (
	uploadDirMode = 0o750
	mp4BoxHeader  = int64(8)
	mp4FileType   = "ftyp"
)

// UploadBlobs stores resumable chunks under the durable application data
// directory and atomically publishes assembled media files.
type UploadBlobs struct {
	dataDir string
}

func NewUploadBlobs(dataDir string) *UploadBlobs {
	return &UploadBlobs{dataDir: dataDir}
}

func (s *UploadBlobs) WriteChunk(
	sessionID string,
	assetID string,
	index int,
	expectedSize int64,
	expectedSHA256 string,
	body io.Reader,
) (int64, string, error) {
	if !safeSegment(sessionID) || !safeSegment(assetID) || index < 0 || expectedSize <= 0 {
		return 0, "", domain.ErrInvalidUpload
	}
	dir := s.assetDir(sessionID, assetID)
	if err := os.MkdirAll(dir, uploadDirMode); err != nil {
		return 0, "", fmt.Errorf("create upload chunk directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".chunk-")
	if err != nil {
		return 0, "", fmt.Errorf("create upload chunk: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}()

	hasher := sha256.New()
	written, err := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(body, expectedSize+1))
	if err != nil {
		return 0, "", fmt.Errorf("read upload chunk: %w", err)
	}
	if written != expectedSize {
		return 0, "", fmt.Errorf("%w: chunk size %d, expected %d", domain.ErrInvalidUpload, written, expectedSize)
	}
	if err := tmp.Sync(); err != nil {
		return 0, "", fmt.Errorf("sync upload chunk: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return 0, "", fmt.Errorf("close upload chunk: %w", err)
	}

	digest := hex.EncodeToString(hasher.Sum(nil))
	if expectedSHA256 != "" && !strings.EqualFold(expectedSHA256, digest) {
		return 0, "", fmt.Errorf("%w: sha256 mismatch", domain.ErrInvalidUpload)
	}
	chunkPath := filepath.Join(dir, fmt.Sprintf("%08d.part", index))
	if _, statErr := os.Stat(chunkPath); statErr == nil {
		return verifyExistingChunk(chunkPath, written, digest)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return 0, "", fmt.Errorf("check upload chunk: %w", statErr)
	}
	if err := os.Link(tmpPath, chunkPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return verifyExistingChunk(chunkPath, written, digest)
		}
		return 0, "", fmt.Errorf("publish upload chunk: %w", err)
	}
	return written, digest, nil
}

func verifyExistingChunk(path string, expectedSize int64, expectedDigest string) (int64, string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, "", fmt.Errorf("check existing upload chunk: %w", err)
	}
	if info.Size() != expectedSize {
		return 0, "", domain.ErrChunkConflict
	}
	digest, err := hashPath(path)
	if err != nil {
		return 0, "", fmt.Errorf("hash existing upload chunk: %w", err)
	}
	if digest != expectedDigest {
		return 0, "", domain.ErrChunkConflict
	}
	return expectedSize, expectedDigest, nil
}

func (s *UploadBlobs) Stage(sessionID, assetID, mediaID string, chunks []domain.UploadChunk) (*port.StagedUpload, error) {
	if !safeSegment(sessionID) || !safeSegment(assetID) || !safeSegment(mediaID) {
		return nil, domain.ErrInvalidUpload
	}
	dir := filepath.Join(s.dataDir, "media", mediaID)
	if err := os.MkdirAll(dir, uploadDirMode); err != nil {
		return nil, fmt.Errorf("create media directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".asset-")
	if err != nil {
		return nil, fmt.Errorf("create assembled media: %w", err)
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	hasher := sha256.New()
	var total int64
	for _, chunk := range chunks {
		chunkPath := filepath.Join(s.assetDir(sessionID, assetID), fmt.Sprintf("%08d.part", chunk.Index))
		file, openErr := os.Open(chunkPath)
		if openErr != nil {
			return nil, fmt.Errorf("open upload chunk %d: %w", chunk.Index, openErr)
		}
		written, copyErr := io.Copy(io.MultiWriter(tmp, hasher), io.LimitReader(file, chunk.Size+1))
		closeErr := file.Close()
		if copyErr != nil {
			return nil, fmt.Errorf("assemble upload chunk %d: %w", chunk.Index, copyErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close upload chunk %d: %w", chunk.Index, closeErr)
		}
		if written != chunk.Size {
			return nil, fmt.Errorf("%w: assembled chunk %d has size %d, expected %d", domain.ErrInvalidUpload, chunk.Index, written, chunk.Size)
		}
		total += written
	}
	if syncErr := tmp.Sync(); syncErr != nil {
		return nil, fmt.Errorf("sync assembled media: %w", syncErr)
	}
	if closeErr := tmp.Close(); closeErr != nil {
		return nil, fmt.Errorf("close assembled media: %w", closeErr)
	}
	mime, fastStart, err := inspectMedia(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("inspect assembled media: %w", err)
	}
	keep = true
	return &port.StagedUpload{Path: tmpPath, SHA256: hex.EncodeToString(hasher.Sum(nil)), MIME: mime, Size: total, FastStart: fastStart}, nil
}

func (s *UploadBlobs) Publish(stagedPath, mediaID, filename string) (string, error) {
	if !safeSegment(mediaID) || !safeSegment(filename) {
		return "", domain.ErrInvalidUpload
	}
	finalPath := filepath.Join(s.dataDir, "media", mediaID, filename)
	if err := os.Rename(stagedPath, finalPath); err != nil {
		return "", fmt.Errorf("publish assembled media: %w", err)
	}
	return finalPath, nil
}

func (*UploadBlobs) Discard(path string) error { return os.Remove(path) }

func (s *UploadBlobs) RemoveAsset(sessionID, assetID string) error {
	if !safeSegment(sessionID) || !safeSegment(assetID) {
		return domain.ErrInvalidUpload
	}
	return os.RemoveAll(s.assetDir(sessionID, assetID))
}

func (s *UploadBlobs) RemoveSession(sessionID string) error {
	if !safeSegment(sessionID) {
		return domain.ErrInvalidUpload
	}
	return os.RemoveAll(filepath.Join(s.dataDir, "uploads", sessionID))
}

func (s *UploadBlobs) assetDir(sessionID, assetID string) string {
	return filepath.Join(s.dataDir, "uploads", sessionID, assetID)
}

func safeSegment(value string) bool {
	return value != "" && filepath.Base(value) == value && value != "." && value != ".."
}

func hashPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hasher.Sum(nil)), nil
}

func inspectMedia(path string) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = file.Close() }()
	buf := make([]byte, 512)
	n, err := io.ReadFull(file, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", false, err
	}
	buf = buf[:n]
	if len(buf) >= 12 && string(buf[4:8]) == mp4FileType {
		mime := "video/mp4"
		if string(buf[8:12]) == "qt  " {
			mime = "video/quicktime"
		}
		fastStart, inspectErr := mp4FastStart(file)
		return mime, fastStart, inspectErr
	}
	if len(buf) >= 4 && string(buf[:4]) == "\x1a\x45\xdf\xa3" {
		return "video/webm", false, nil
	}
	return http.DetectContentType(buf), false, nil
}

func mp4FastStart(file *os.File) (bool, error) {
	info, err := file.Stat()
	if err != nil {
		return false, err
	}
	var moovOffset int64 = -1
	var mdatOffset int64 = -1
	for offset := int64(0); offset+mp4BoxHeader <= info.Size(); {
		var header [16]byte
		if _, err := file.ReadAt(header[:8], offset); err != nil {
			return false, err
		}
		boxSize := int64(binary.BigEndian.Uint32(header[:4]))
		headerSize := mp4BoxHeader
		switch boxSize {
		case 1:
			if _, err := file.ReadAt(header[8:16], offset+mp4BoxHeader); err != nil {
				return false, err
			}
			extendedSize := binary.BigEndian.Uint64(header[8:16])
			if extendedSize > math.MaxInt64 {
				return false, domain.ErrInvalidUpload
			}
			boxSize = int64(extendedSize)
			headerSize = 16
		case 0:
			boxSize = info.Size() - offset
		}
		if boxSize < headerSize || boxSize > info.Size()-offset {
			return false, domain.ErrInvalidUpload
		}
		switch string(header[4:8]) {
		case "moov":
			moovOffset = offset
		case "mdat":
			mdatOffset = offset
		}
		offset += boxSize
	}
	return moovOffset >= 0 && (mdatOffset < 0 || moovOffset < mdatOffset), nil
}

var _ port.UploadBlobStore = (*UploadBlobs)(nil)
