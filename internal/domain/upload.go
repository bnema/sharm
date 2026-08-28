package domain

import "time"

// AssetRole identifies the purpose of a file associated with a media item.
type AssetRole string

const (
	AssetRolePrimaryH264     AssetRole = "primary-h264"
	AssetRoleOriginal        AssetRole = "original"
	AssetRoleSourceTransient AssetRole = "source-transient"
)

func (r AssetRole) Valid() bool {
	switch r {
	case AssetRolePrimaryH264, AssetRoleOriginal, AssetRoleSourceTransient:
		return true
	default:
		return false
	}
}

// AssetStatus is the durable state of a published or upload asset.
type AssetStatus string

const (
	AssetStatusUploading  AssetStatus = "uploading"
	AssetStatusFinalizing AssetStatus = "finalizing"
	AssetStatusAvailable  AssetStatus = "available"
	AssetStatusFailed     AssetStatus = "failed"
	AssetStatusCanceled   AssetStatus = "canceled"
)

// UploadSessionStatus is independent from the media publication status. A
// session may still be uploading an optional original after the primary media
// variant is already available.
type UploadSessionStatus string

const (
	UploadSessionStatusActive    UploadSessionStatus = "active"
	UploadSessionStatusCompleted UploadSessionStatus = "completed"
	UploadSessionStatusExpired   UploadSessionStatus = "expired"
	UploadSessionStatusCanceled  UploadSessionStatus = "canceled"
	UploadSessionStatusFailed    UploadSessionStatus = "failed"
)

type MediaAsset struct {
	ID           string
	MediaID      string
	Role         AssetRole
	Filename     string
	Path         string
	Size         int64
	SHA256       string
	Status       AssetStatus
	ErrorMessage string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type UploadSession struct {
	ID            string
	MediaID       string
	UserID        int64
	Filename      string
	RetentionDays int
	KeepOriginal  bool
	ExpectedBytes int64
	ReservedBytes int64
	Status        UploadSessionStatus
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	Assets        []UploadAsset
}

type UploadAsset struct {
	ID             string
	SessionID      string
	MediaID        string
	Role           AssetRole
	Filename       string
	ExpectedSize   int64
	ChunkSize      int64
	TotalChunks    int
	ReceivedBytes  int64
	ExpectedSHA256 string
	SHA256         string
	Status         AssetStatus
	Path           string
	ErrorMessage   string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
	Chunks         []UploadChunk
}

type UploadChunk struct {
	AssetID   string
	Index     int
	Size      int64
	SHA256    string
	CreatedAt time.Time
}

func (a *UploadAsset) IsComplete() bool {
	return a.Status == AssetStatusAvailable
}

func (s *UploadSession) IsExpired(now time.Time) bool {
	return !now.Before(s.ExpiresAt)
}

func (s *UploadSession) RefreshStatus() {
	if s.Status != UploadSessionStatusActive {
		return
	}
	for i := range s.Assets {
		asset := &s.Assets[i]
		if asset.Role == AssetRolePrimaryH264 && (asset.Status == AssetStatusFailed || asset.Status == AssetStatusCanceled) {
			s.Status = UploadSessionStatusFailed
			return
		}
		if asset.Role == AssetRolePrimaryH264 && asset.Status != AssetStatusAvailable {
			return
		}
	}
	// Optional originals are deliberately non-blocking: a failed original
	// upload must never revoke an already published primary variant.
	for i := range s.Assets {
		asset := &s.Assets[i]
		if asset.Role == AssetRolePrimaryH264 {
			continue
		}
		if asset.Status == AssetStatusUploading || asset.Status == AssetStatusFinalizing {
			return
		}
	}
	s.Status = UploadSessionStatusCompleted
}
