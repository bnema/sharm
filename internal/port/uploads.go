package port

import (
	"io"
	"time"

	"github.com/bnema/sharm/internal/domain"
)

// StagedUpload describes an assembled but not yet published upload.
type StagedUpload struct {
	Path      string
	SHA256    string
	MIME      string
	Size      int64
	FastStart bool
}

// UploadBlobStore owns the physical chunk layout and atomic publication of
// resumable upload bytes. The application layer controls lifecycle and policy.
type UploadBlobStore interface {
	WriteChunk(
		sessionID, assetID string,
		index int,
		expectedSize int64,
		expectedSHA256 string,
		body io.Reader,
	) (size int64, sha256 string, err error)
	Stage(sessionID, assetID, mediaID string, chunks []domain.UploadChunk) (*StagedUpload, error)
	Publish(stagedPath, mediaID, filename string) (string, error)
	Discard(path string) error
	RemoveAsset(sessionID, assetID string) error
	RemoveSession(sessionID string) error
	RemoveMedia(mediaID string) error
}

// UploadStore persists resumable upload metadata and state transitions.
type UploadStore interface {
	CreateUploadSession(session *domain.UploadSession, assets []domain.UploadAsset, maxReservedBytes int64) error
	GetUploadSession(id string) (*domain.UploadSession, error)
	GetUploadAsset(id string) (*domain.UploadAsset, error)
	ListUploadChunks(assetID string) ([]domain.UploadChunk, error)
	RecordUploadChunk(assetID string, chunk *domain.UploadChunk) (inserted bool, err error)
	ClaimUploadAssetFinalization(id string, now time.Time) (bool, error)
	ReleaseUploadAssetFinalization(id, errMsg string, now time.Time) error
	CompleteUploadAsset(id, path, sha256 string, receivedBytes int64, completedAt time.Time) error
	FailUploadAsset(id, errMsg string, now time.Time) error
	UpdateUploadSessionStatus(id string, status domain.UploadSessionStatus, now time.Time) error
	ListExpiredUploadSessions(now time.Time) ([]domain.UploadSession, error)
	DeleteUploadSession(id string) error
}
