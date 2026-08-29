package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/infrastructure/logger"
	"github.com/bnema/sharm/internal/service"
)

type createUploadRequest struct {
	Filename               string `json:"filename"`
	PrimaryFilename        string `json:"primary_filename"`
	PrimarySize            int64  `json:"primary_size"`
	PrimarySHA256          string `json:"primary_sha256"`
	OriginalFilename       string `json:"original_filename"`
	OriginalSize           int64  `json:"original_size"`
	OriginalSHA256         string `json:"original_sha256"`
	KeepOriginal           bool   `json:"keep_original"`
	ReusePrimaryAsOriginal bool   `json:"reuse_primary_as_original"`
	RetentionDays          int    `json:"retention_days"`
}

type ResumableUploadService interface {
	GetChunkSize() int64
	CreateSession(service.CreateUploadInput) (*domain.UploadSession, error)
	GetSession(userID int64, sessionID string) (*domain.UploadSession, error)
	WriteChunk(userID int64, sessionID, assetID string, index int, expectedSHA256 string, body io.Reader) (*domain.UploadChunk, error)
	FinalizeAsset(userID int64, sessionID, assetID string) (*service.FinalizeUploadResult, error)
	CancelSession(userID int64, sessionID string) error
}

type uploadSessionResponse struct {
	Session uploadSessionDTO `json:"session"`
}

type uploadFinalizeResponse struct {
	Session uploadSessionDTO `json:"session"`
	Asset   uploadAssetDTO   `json:"asset"`
	MediaID string           `json:"media_id"`
	Variant *variantDTO      `json:"variant,omitempty"`
}

type uploadSessionDTO struct {
	ID            string                     `json:"id"`
	MediaID       string                     `json:"media_id"`
	Filename      string                     `json:"filename"`
	RetentionDays int                        `json:"retention_days"`
	KeepOriginal  bool                       `json:"keep_original"`
	ExpectedBytes int64                      `json:"expected_bytes"`
	ReservedBytes int64                      `json:"reserved_bytes"`
	Status        domain.UploadSessionStatus `json:"status"`
	ExpiresAt     time.Time                  `json:"expires_at"`
	CreatedAt     time.Time                  `json:"created_at"`
	UpdatedAt     time.Time                  `json:"updated_at"`
	Assets        []uploadAssetDTO           `json:"assets"`
}

type uploadAssetDTO struct {
	ID            string             `json:"id"`
	SessionID     string             `json:"session_id"`
	MediaID       string             `json:"media_id"`
	Role          domain.AssetRole   `json:"role"`
	Filename      string             `json:"filename"`
	ExpectedSize  int64              `json:"expected_size"`
	ChunkSize     int64              `json:"chunk_size"`
	TotalChunks   int                `json:"total_chunks"`
	ReceivedBytes int64              `json:"received_bytes"`
	Status        domain.AssetStatus `json:"status"`
	ErrorMessage  string             `json:"error_message,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	CompletedAt   *time.Time         `json:"completed_at,omitempty"`
	Chunks        []uploadChunkDTO   `json:"chunks,omitempty"`
}

type uploadChunkDTO struct {
	Index     int       `json:"index"`
	Size      int64     `json:"size"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

type variantDTO struct {
	ID              int64                `json:"id"`
	MediaID         string               `json:"media_id"`
	Codec           domain.Codec         `json:"codec"`
	Container       string               `json:"container"`
	VideoCodec      string               `json:"video_codec"`
	AudioCodec      string               `json:"audio_codec"`
	HasAudio        bool                 `json:"has_audio"`
	Profile         string               `json:"profile,omitempty"`
	Level           string               `json:"level,omitempty"`
	MIMEType        string               `json:"mime_type"`
	Origin          domain.VariantOrigin `json:"origin"`
	Primary         bool                 `json:"primary"`
	Progress        int                  `json:"progress"`
	DurationSeconds float64              `json:"duration_seconds"`
	FileSize        int64                `json:"file_size"`
	Width           int                  `json:"width"`
	Height          int                  `json:"height"`
	Status          domain.VariantStatus `json:"status"`
	ErrorMessage    string               `json:"error_message,omitempty"`
	CreatedAt       time.Time            `json:"created_at"`
}

func (h *Handlers) CreateUploadSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.uploadSvc == nil {
			http.Error(w, "resumable uploads are unavailable", http.StatusNotImplemented)
			return
		}
		user, ok := authenticatedUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var request createUploadRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&request); err != nil {
			writeUploadError(w, r, fmt.Errorf("%w: decode upload session: %w", domain.ErrInvalidUpload, err))
			return
		}
		session, err := h.uploadSvc.CreateSession(service.CreateUploadInput{
			UserID:                 user.ID,
			Filename:               request.Filename,
			PrimaryFilename:        request.PrimaryFilename,
			PrimarySize:            request.PrimarySize,
			PrimarySHA256:          request.PrimarySHA256,
			OriginalFilename:       request.OriginalFilename,
			OriginalSize:           request.OriginalSize,
			OriginalSHA256:         request.OriginalSHA256,
			KeepOriginal:           request.KeepOriginal,
			ReusePrimaryAsOriginal: request.ReusePrimaryAsOriginal,
			RetentionDays:          request.RetentionDays,
		})
		if err != nil {
			writeUploadError(w, r, err)
			return
		}
		writeJSON(w, http.StatusCreated, uploadSessionResponse{Session: newUploadSessionDTO(session)})
	}
}

func (h *Handlers) GetUploadSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.uploadSvc == nil {
			http.Error(w, "resumable uploads are unavailable", http.StatusNotImplemented)
			return
		}
		user, ok := authenticatedUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		session, err := h.uploadSvc.GetSession(user.ID, r.PathValue("sessionID"))
		if err != nil {
			writeUploadError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, uploadSessionResponse{Session: newUploadSessionDTO(session)})
	}
}

func (h *Handlers) UploadSessionChunk() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.uploadSvc == nil {
			http.Error(w, "resumable uploads are unavailable", http.StatusNotImplemented)
			return
		}
		user, ok := authenticatedUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		index, err := strconv.Atoi(r.PathValue("index"))
		if err != nil || index < 0 {
			writeUploadError(w, r, domain.ErrInvalidUpload)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, h.uploadSvc.GetChunkSize()+1)
		chunk, err := h.uploadSvc.WriteChunk(
			user.ID,
			r.PathValue("sessionID"),
			r.PathValue("assetID"),
			index,
			r.Header.Get("X-Chunk-SHA256"),
			r.Body,
		)
		if err != nil {
			writeUploadError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, newUploadChunkDTO(*chunk))
	}
}

func (h *Handlers) CompleteUploadAsset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.uploadSvc == nil {
			http.Error(w, "resumable uploads are unavailable", http.StatusNotImplemented)
			return
		}
		user, ok := authenticatedUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		result, err := h.uploadSvc.FinalizeAsset(
			user.ID,
			r.PathValue("sessionID"),
			r.PathValue("assetID"),
		)
		if err != nil {
			writeUploadError(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, uploadFinalizeResponse{
			Session: newUploadSessionDTO(result.Session),
			Asset:   newUploadAssetDTO(*result.Asset),
			MediaID: result.Media.ID,
			Variant: newVariantDTO(result.Variant),
		})
	}
}

func (h *Handlers) CancelUploadSession() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.uploadSvc == nil {
			http.Error(w, "resumable uploads are unavailable", http.StatusNotImplemented)
			return
		}
		user, ok := authenticatedUser(r)
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := h.uploadSvc.CancelSession(user.ID, r.PathValue("sessionID")); err != nil {
			writeUploadError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func newUploadSessionDTO(session *domain.UploadSession) uploadSessionDTO {
	assets := make([]uploadAssetDTO, len(session.Assets))
	for i := range session.Assets {
		assets[i] = newUploadAssetDTO(session.Assets[i])
	}
	return uploadSessionDTO{
		ID:            session.ID,
		MediaID:       session.MediaID,
		Filename:      session.Filename,
		RetentionDays: session.RetentionDays,
		KeepOriginal:  session.KeepOriginal,
		ExpectedBytes: session.ExpectedBytes,
		ReservedBytes: session.ReservedBytes,
		Status:        session.Status,
		ExpiresAt:     session.ExpiresAt,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.UpdatedAt,
		Assets:        assets,
	}
}

func newUploadAssetDTO(asset domain.UploadAsset) uploadAssetDTO {
	chunks := make([]uploadChunkDTO, len(asset.Chunks))
	for i := range asset.Chunks {
		chunks[i] = newUploadChunkDTO(asset.Chunks[i])
	}
	return uploadAssetDTO{
		ID:            asset.ID,
		SessionID:     asset.SessionID,
		MediaID:       asset.MediaID,
		Role:          asset.Role,
		Filename:      asset.Filename,
		ExpectedSize:  asset.ExpectedSize,
		ChunkSize:     asset.ChunkSize,
		TotalChunks:   asset.TotalChunks,
		ReceivedBytes: asset.ReceivedBytes,
		Status:        asset.Status,
		ErrorMessage:  asset.ErrorMessage,
		CreatedAt:     asset.CreatedAt,
		UpdatedAt:     asset.UpdatedAt,
		CompletedAt:   asset.CompletedAt,
		Chunks:        chunks,
	}
}

func newUploadChunkDTO(chunk domain.UploadChunk) uploadChunkDTO {
	return uploadChunkDTO{Index: chunk.Index, Size: chunk.Size, SHA256: chunk.SHA256, CreatedAt: chunk.CreatedAt}
}

func newVariantDTO(variant *domain.Variant) *variantDTO {
	if variant == nil {
		return nil
	}
	return &variantDTO{
		ID:              variant.ID,
		MediaID:         variant.MediaID,
		Codec:           variant.Codec,
		Container:       variant.Container,
		VideoCodec:      variant.VideoCodec,
		AudioCodec:      variant.AudioCodec,
		HasAudio:        variant.HasAudio,
		Profile:         variant.Profile,
		Level:           variant.Level,
		MIMEType:        variant.MIMEType,
		Origin:          variant.Origin,
		Primary:         variant.Primary,
		Progress:        variant.Progress,
		DurationSeconds: variant.DurationSeconds,
		FileSize:        variant.FileSize,
		Width:           variant.Width,
		Height:          variant.Height,
		Status:          variant.Status,
		ErrorMessage:    variant.ErrorMessage,
		CreatedAt:       variant.CreatedAt,
	}
}

func authenticatedUser(r *http.Request) (*domain.User, bool) {
	user, ok := r.Context().Value(userKey).(*domain.User)
	return user, ok && user != nil && user.ID > 0
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeUploadError(w http.ResponseWriter, r *http.Request, err error) {
	status := http.StatusInternalServerError
	message := "upload failed"
	var maxBytesErr *http.MaxBytesError
	switch {
	case errors.As(err, &maxBytesErr):
		status = http.StatusRequestEntityTooLarge
		message = "upload chunk exceeds the configured limit"
	case errors.Is(err, domain.ErrPermission), errors.Is(err, domain.ErrUploadOwnership):
		status = http.StatusNotFound
		message = "upload not found"
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		message = "upload not found"
	case errors.Is(err, domain.ErrInvalidUpload), errors.Is(err, domain.ErrUnsupportedMedia):
		status = http.StatusUnprocessableEntity
		message = "invalid upload"
	case errors.Is(err, domain.ErrQuotaExceeded):
		status = http.StatusRequestEntityTooLarge
		message = "upload exceeds the configured limit"
	case errors.Is(err, domain.ErrChunkConflict):
		status = http.StatusConflict
		message = "chunk conflicts with an earlier retry"
	case errors.Is(err, domain.ErrUploadIncomplete):
		status = http.StatusConflict
		message = "upload is incomplete"
	case errors.Is(err, domain.ErrUploadExpired), errors.Is(err, domain.ErrExpired):
		status = http.StatusGone
		message = "upload session expired"
	}
	if status == http.StatusInternalServerError {
		logger.Error.Printf(
			"resumable upload failed session=%s asset=%s err=%v",
			logger.SanitizeForLog(r.PathValue("sessionID")),
			logger.SanitizeForLog(r.PathValue("assetID")),
			err,
		)
	}
	writeJSON(w, status, map[string]string{"error": message})
}
