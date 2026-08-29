package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

const (
	DefaultUploadChunkSize       = int64(4 * 1024 * 1024)
	DefaultUploadSessionTTL      = 24 * time.Hour
	DefaultUploadMaxAssetBytes   = int64(20 * 1024 * 1024 * 1024)
	DefaultUploadMaxSessionBytes = int64(40 * 1024 * 1024 * 1024)
	DefaultUploadReservedBytes   = int64(40 * 1024 * 1024 * 1024)
	uploadMP4MIME                = "video/mp4"
	uploadMP4Container           = "mp4"
	uploadWebMFormat             = "matroska,webm"
	probeVideoStream             = "video"
	serverFallbackFPS            = 30
	uploadProbeTimeout           = 30 * time.Second
)

type UploadConfig struct {
	ChunkSize        int64
	SessionTTL       time.Duration
	MaxAssetBytes    int64
	MaxSessionBytes  int64
	MaxReservedBytes int64
}

type CreateUploadInput struct {
	UserID                 int64
	Filename               string
	PrimaryFilename        string
	PrimarySize            int64
	PrimarySHA256          string
	OriginalFilename       string
	OriginalSize           int64
	OriginalSHA256         string
	KeepOriginal           bool
	ReusePrimaryAsOriginal bool
	RetentionDays          int
}

type FinalizeUploadResult struct {
	Session *domain.UploadSession
	Asset   *domain.UploadAsset
	Media   *domain.Media
	Variant *domain.Variant
}

type uploadMediaProbe interface {
	ProbeContext(ctx context.Context, path string) (*domain.ProbeResult, error)
}

type uploadJobQueue interface {
	Enqueue(mediaID string, jobType domain.JobType, codec domain.Codec, fps int) (*domain.Job, error)
	GetActive(mediaID string, jobType domain.JobType, codec domain.Codec) (*domain.Job, error)
}

type UploadService struct {
	uploads     port.UploadStore
	media       port.MediaStore
	converter   uploadMediaProbe
	blobs       port.UploadBlobStore
	chunkSize   int64
	ttl         time.Duration
	maxAsset    int64
	maxTotal    int64
	maxReserved int64
	log         port.Logger
	jobs        uploadJobQueue
	now         func() time.Time
}

func NewUploadService(
	uploads port.UploadStore,
	media port.MediaStore,
	converter uploadMediaProbe,
	blobs port.UploadBlobStore,
	cfg UploadConfig,
	log port.Logger,
	jobs uploadJobQueue,
) *UploadService {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = DefaultUploadChunkSize
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = DefaultUploadSessionTTL
	}
	if cfg.MaxAssetBytes <= 0 {
		cfg.MaxAssetBytes = DefaultUploadMaxAssetBytes
	}
	if cfg.MaxSessionBytes <= 0 {
		cfg.MaxSessionBytes = DefaultUploadMaxSessionBytes
	}
	if cfg.MaxReservedBytes <= 0 {
		cfg.MaxReservedBytes = DefaultUploadReservedBytes
	}
	return &UploadService{
		uploads:     uploads,
		media:       media,
		converter:   converter,
		blobs:       blobs,
		chunkSize:   cfg.ChunkSize,
		ttl:         cfg.SessionTTL,
		maxAsset:    cfg.MaxAssetBytes,
		maxTotal:    cfg.MaxSessionBytes,
		maxReserved: cfg.MaxReservedBytes,
		log:         log,
		jobs:        jobs,
		now:         time.Now,
	}
}

func (s *UploadService) ChunkSize() int64 {
	return s.chunkSize
}

func (s *UploadService) CreateSession(input CreateUploadInput) (*domain.UploadSession, error) {
	if input.UserID <= 0 {
		return nil, domain.ErrPermission
	}
	total, err := s.validateCreateUploadInput(&input)
	if err != nil {
		return nil, err
	}
	if total > s.maxReserved {
		return nil, domain.ErrQuotaExceeded
	}

	now := s.now()
	media := domain.NewMedia(domain.MediaTypeVideo, input.Filename, "", input.RetentionDays)
	media.CreatedAt = now
	media.ExpiresAt = now.AddDate(0, 0, input.RetentionDays)
	if err := s.media.Save(media); err != nil {
		return nil, fmt.Errorf("create media for upload: %w", err)
	}

	session := &domain.UploadSession{
		ID:            newUploadID("session"),
		MediaID:       media.ID,
		UserID:        input.UserID,
		Filename:      input.Filename,
		RetentionDays: input.RetentionDays,
		KeepOriginal:  input.KeepOriginal,
		ExpectedBytes: total,
		ReservedBytes: total,
		Status:        domain.UploadSessionStatusActive,
		ExpiresAt:     now.Add(s.ttl),
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	assets := []domain.UploadAsset{
		newUploadAsset(session, domain.AssetRolePrimaryH264, input.PrimaryFilename, input.PrimarySize, input.PrimarySHA256, s.chunkSize, now),
	}
	if input.KeepOriginal && !input.ReusePrimaryAsOriginal {
		assets = append(assets, newUploadAsset(
			session,
			domain.AssetRoleOriginal,
			input.OriginalFilename,
			input.OriginalSize,
			input.OriginalSHA256,
			s.chunkSize,
			now,
		))
	}
	if err := s.uploads.CreateUploadSession(session, assets, s.maxReserved); err != nil {
		if deleteErr := s.media.Delete(media.ID); deleteErr != nil {
			s.log.Warnf("delete media after upload session failure media=%s err=%v", media.ID, deleteErr)
		}
		return nil, fmt.Errorf("create upload session: %w", err)
	}
	session.Assets = assets
	return session, nil
}

func (s *UploadService) validateCreateUploadInput(input *CreateUploadInput) (int64, error) {
	filename, err := normalizeUploadFilename(input.Filename)
	if err != nil {
		return 0, err
	}
	input.Filename = filename
	if input.PrimaryFilename == "" {
		input.PrimaryFilename = input.Filename
	}
	primaryFilename, err := normalizeUploadFilename(input.PrimaryFilename)
	if err != nil {
		return 0, err
	}
	input.PrimaryFilename = primaryFilename
	if input.PrimarySize <= 0 || input.PrimarySize > s.maxAsset {
		return 0, domain.ErrQuotaExceeded
	}
	if input.KeepOriginal && !input.ReusePrimaryAsOriginal {
		if input.OriginalFilename == "" {
			input.OriginalFilename = input.Filename
		}
		originalFilename, err := normalizeUploadFilename(input.OriginalFilename)
		if err != nil {
			return 0, err
		}
		input.OriginalFilename = originalFilename
		if input.OriginalSize <= 0 || input.OriginalSize > s.maxAsset {
			return 0, domain.ErrQuotaExceeded
		}
	}
	if input.RetentionDays <= 0 {
		input.RetentionDays = 7
	}
	total := input.PrimarySize
	if input.KeepOriginal && !input.ReusePrimaryAsOriginal {
		total += input.OriginalSize
	}
	if total <= 0 || total > s.maxTotal {
		return 0, domain.ErrQuotaExceeded
	}
	return total, nil
}

func (s *UploadService) GetSession(userID int64, sessionID string) (*domain.UploadSession, error) {
	session, err := s.uploads.GetUploadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if session.UserID != userID {
		return nil, domain.ErrUploadOwnership
	}
	if session.IsExpired(s.now()) && session.Status == domain.UploadSessionStatusActive {
		_ = s.uploads.UpdateUploadSessionStatus(session.ID, domain.UploadSessionStatusExpired, s.now())
		session.Status = domain.UploadSessionStatusExpired
	}
	return session, nil
}

func (s *UploadService) WriteChunk(
	userID int64,
	sessionID, assetID string,
	index int,
	expectedSHA256 string,
	body io.Reader,
) (*domain.UploadChunk, error) {
	session, err := s.GetSession(userID, sessionID)
	if err != nil {
		return nil, err
	}
	if session.Status != domain.UploadSessionStatusActive || session.IsExpired(s.now()) {
		return nil, domain.ErrUploadExpired
	}
	asset, err := s.uploads.GetUploadAsset(assetID)
	if err != nil {
		return nil, err
	}
	if asset.SessionID != session.ID || asset.MediaID != session.MediaID {
		return nil, domain.ErrUploadOwnership
	}
	if asset.Status != domain.AssetStatusUploading {
		return nil, domain.ErrInvalidUpload
	}
	if index < 0 || index >= asset.TotalChunks {
		return nil, domain.ErrInvalidUpload
	}
	expectedSize := min(asset.ExpectedSize-int64(index)*asset.ChunkSize, asset.ChunkSize)
	if expectedSize <= 0 {
		return nil, domain.ErrInvalidUpload
	}

	written, actualSHA256, err := s.blobs.WriteChunk(session.ID, asset.ID, index, expectedSize, expectedSHA256, body)
	if err != nil {
		return nil, err
	}
	chunk := &domain.UploadChunk{AssetID: asset.ID, Index: index, Size: written, SHA256: actualSHA256, CreatedAt: s.now()}
	if _, err := s.uploads.RecordUploadChunk(asset.ID, chunk); err != nil {
		return nil, err
	}
	return chunk, nil
}

func (s *UploadService) FinalizeAsset(userID int64, sessionID, assetID string) (*FinalizeUploadResult, error) {
	session, asset, chunks, existing, err := s.prepareFinalization(userID, sessionID, assetID)
	if err != nil || existing != nil {
		return existing, err
	}
	publishedPath, sha256sum, err := s.finalizeBytes(session, asset, chunks)
	if err != nil {
		s.recordFinalizationFailure(asset.ID, err)
		return nil, err
	}
	if completeErr := s.completeFinalization(asset, publishedPath, sha256sum); completeErr != nil {
		_ = s.uploads.ReleaseUploadAssetFinalization(asset.ID, completeErr.Error(), s.now())
		return nil, completeErr
	}
	if cleanupErr := s.blobs.RemoveAsset(session.ID, asset.ID); cleanupErr != nil {
		s.log.Warnf("remove completed upload chunks asset=%s err=%v", asset.ID, cleanupErr)
	}
	return s.finalizationResult(userID, session, asset)
}

func (s *UploadService) prepareFinalization(
	userID int64,
	sessionID string,
	assetID string,
) (*domain.UploadSession, *domain.UploadAsset, []domain.UploadChunk, *FinalizeUploadResult, error) {
	session, err := s.GetSession(userID, sessionID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	asset, err := s.uploads.GetUploadAsset(assetID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if asset.SessionID != session.ID || asset.MediaID != session.MediaID {
		return nil, nil, nil, nil, domain.ErrUploadOwnership
	}
	if asset.Status == domain.AssetStatusAvailable {
		result, resultErr := s.resultForAvailable(session, asset)
		return nil, nil, nil, result, resultErr
	}
	if session.Status != domain.UploadSessionStatusActive || session.IsExpired(s.now()) {
		return nil, nil, nil, nil, domain.ErrUploadExpired
	}
	if asset.Status != domain.AssetStatusUploading && asset.Status != domain.AssetStatusFinalizing {
		return nil, nil, nil, nil, domain.ErrInvalidUpload
	}
	chunks, err := s.uploads.ListUploadChunks(asset.ID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("list upload chunks for finalization: %w", err)
	}
	if validationErr := validateChunkSet(asset, chunks); validationErr != nil {
		return nil, nil, nil, nil, validationErr
	}
	claimed, err := s.uploads.ClaimUploadAssetFinalization(asset.ID, s.now())
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("claim upload finalization: %w", err)
	}
	if claimed {
		return session, asset, chunks, nil, nil
	}
	asset, err = s.uploads.GetUploadAsset(asset.ID)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	if asset.Status == domain.AssetStatusAvailable {
		result, resultErr := s.resultForAvailable(session, asset)
		return nil, nil, nil, result, resultErr
	}
	return nil, nil, nil, nil, domain.ErrChunkConflict
}

func (s *UploadService) finalizeBytes(
	session *domain.UploadSession,
	asset *domain.UploadAsset,
	chunks []domain.UploadChunk,
) (string, string, error) {
	staged, validation, err := s.stageAndValidate(session, asset, chunks)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = s.blobs.Discard(staged.Path) }()
	publishedPath, err := s.blobs.Publish(staged.Path, asset.MediaID, finalAssetName(asset, validation.primaryReady))
	if err != nil {
		return "", "", err
	}
	if publishErr := s.publishAsset(session, asset, publishedPath, staged.SHA256, validation); publishErr != nil {
		_ = s.blobs.Discard(publishedPath)
		return "", "", publishErr
	}
	return publishedPath, staged.SHA256, nil
}

func (s *UploadService) completeFinalization(asset *domain.UploadAsset, path, sha256sum string) error {
	completeErr := s.uploads.CompleteUploadAsset(asset.ID, path, sha256sum, asset.ExpectedSize, s.now())
	if completeErr != nil {
		return fmt.Errorf("complete upload asset: %w", completeErr)
	}
	if asset.Role != domain.AssetRoleOriginal {
		return nil
	}
	if updateErr := s.media.UpdateOriginalPath(asset.MediaID, path); updateErr != nil {
		return fmt.Errorf("publish original media path: %w", updateErr)
	}
	return nil
}

func (s *UploadService) finalizationResult(
	userID int64,
	previousSession *domain.UploadSession,
	asset *domain.UploadAsset,
) (*FinalizeUploadResult, error) {
	updatedSession, err := s.GetSession(userID, previousSession.ID)
	if err != nil {
		return nil, err
	}
	updatedSession.RefreshStatus()
	if updatedSession.Status != previousSession.Status {
		updateErr := s.uploads.UpdateUploadSessionStatus(updatedSession.ID, updatedSession.Status, s.now())
		if updateErr != nil {
			return nil, fmt.Errorf("update upload session status: %w", updateErr)
		}
	}
	updatedAsset, err := s.uploads.GetUploadAsset(asset.ID)
	if err != nil {
		return nil, err
	}
	media, err := s.media.Get(asset.MediaID)
	if err != nil {
		return nil, err
	}
	result := &FinalizeUploadResult{Session: updatedSession, Asset: updatedAsset, Media: media}
	if asset.Role == domain.AssetRolePrimaryH264 {
		result.Variant = primaryH264Variant(media)
	}
	return result, nil
}

func primaryH264Variant(media *domain.Media) *domain.Variant {
	for i := range media.Variants {
		if media.Variants[i].Codec == domain.CodecH264 && media.Variants[i].Primary {
			return &media.Variants[i]
		}
	}
	return nil
}

func (s *UploadService) recordFinalizationFailure(assetID string, err error) {
	now := s.now()
	permanent := errors.Is(err, domain.ErrInvalidUpload) || errors.Is(err, domain.ErrUnsupportedMedia)
	if permanent {
		_ = s.uploads.FailUploadAsset(assetID, err.Error(), now)
		return
	}
	_ = s.uploads.ReleaseUploadAssetFinalization(assetID, err.Error(), now)
}

func (s *UploadService) CancelSession(userID int64, sessionID string) error {
	session, err := s.GetSession(userID, sessionID)
	if err != nil {
		return err
	}
	if session.Status != domain.UploadSessionStatusActive {
		return nil
	}
	if err := s.uploads.UpdateUploadSessionStatus(session.ID, domain.UploadSessionStatusCanceled, s.now()); err != nil {
		return err
	}
	if err := s.blobs.RemoveSession(session.ID); err != nil {
		return fmt.Errorf("remove canceled upload: %w", err)
	}
	if err := s.media.Delete(session.MediaID); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return fmt.Errorf("delete canceled upload media: %w", err)
	}
	if err := s.blobs.RemoveMedia(session.MediaID); err != nil {
		return fmt.Errorf("remove canceled media files: %w", err)
	}
	return nil
}

func (s *UploadService) CleanupExpired() error {
	sessions, err := s.uploads.ListExpiredUploadSessions(s.now())
	if err != nil {
		return err
	}
	for i := range sessions {
		session := &sessions[i]
		if err := s.blobs.RemoveSession(session.ID); err != nil {
			s.log.Warnf("remove expired upload files session=%s err=%v", session.ID, err)
		}
		if err := s.uploads.DeleteUploadSession(session.ID); err != nil {
			s.log.Warnf("delete expired upload session session=%s err=%v", session.ID, err)
			continue
		}
		if err := s.media.Delete(session.MediaID); err != nil && !errors.Is(err, domain.ErrNotFound) {
			s.log.Warnf("delete expired upload media media=%s err=%v", session.MediaID, err)
			continue
		}
		if err := s.blobs.RemoveMedia(session.MediaID); err != nil {
			s.log.Warnf("remove expired media files media=%s err=%v", session.MediaID, err)
		}
	}
	return nil
}

type uploadValidation struct {
	probe        *domain.ProbeResult
	primaryReady bool
}

func (s *UploadService) stageAndValidate(
	session *domain.UploadSession,
	asset *domain.UploadAsset,
	chunks []domain.UploadChunk,
) (*port.StagedUpload, *uploadValidation, error) {
	staged, err := s.blobs.Stage(session.ID, asset.ID, session.MediaID, chunks)
	if err != nil {
		return nil, nil, err
	}
	if staged.Size != asset.ExpectedSize || (asset.ExpectedSHA256 != "" && !strings.EqualFold(asset.ExpectedSHA256, staged.SHA256)) {
		_ = s.blobs.Discard(staged.Path)
		return nil, nil, domain.ErrInvalidUpload
	}
	validation, err := s.validateMedia(staged.Path, staged.MIME, staged.FastStart, asset.Role)
	if err != nil {
		_ = s.blobs.Discard(staged.Path)
		return nil, nil, err
	}
	return staged, validation, nil
}

func (s *UploadService) validateMedia(path, mime string, fastStart bool, role domain.AssetRole) (*uploadValidation, error) {
	ctx, cancel := context.WithTimeout(context.Background(), uploadProbeTimeout)
	defer cancel()
	probe, err := s.converter.ProbeContext(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("probe uploaded media: %w", err)
	}
	if probe == nil {
		return nil, domain.ErrUnsupportedMedia
	}
	video := probe.VideoStream()
	if video == nil {
		return nil, fmt.Errorf("%w: video stream is missing", domain.ErrUnsupportedMedia)
	}
	format := strings.Split(probe.Format.FormatName, ",")
	if !allowedOriginalMIME(mime) || !allowedOriginalFormat(format) {
		return nil, fmt.Errorf("%w: media type is not allowed", domain.ErrUnsupportedMedia)
	}
	primaryReady := role == domain.AssetRolePrimaryH264 && isPrimaryCompatible(probe, mime, fastStart, format)
	if video.Width <= 0 || video.Height <= 0 || video.Width > 7680 || video.Height > 4320 {
		return nil, fmt.Errorf("%w: dimensions exceed the upload limit", domain.ErrUnsupportedMedia)
	}
	if duration := domain.ParseDuration(probe.Format.Duration); duration > 6*60*60 {
		return nil, fmt.Errorf("%w: duration exceeds the upload limit", domain.ErrUnsupportedMedia)
	}
	return &uploadValidation{probe: probe, primaryReady: primaryReady}, nil
}

func (s *UploadService) publishAsset(
	session *domain.UploadSession,
	asset *domain.UploadAsset,
	path, sha256sum string,
	validation *uploadValidation,
) error {
	now := s.now()
	if asset.Role == domain.AssetRoleOriginal {
		return s.media.SaveMediaAsset(availableMediaAsset(asset, asset.Role, path, sha256sum, now))
	}
	reuseAsOriginal := sessionReusesPrimaryAsOriginal(session)
	if !validation.primaryReady {
		return s.publishTransientSource(asset, path, sha256sum, now, reuseAsOriginal)
	}
	probe := validation.probe
	video := probe.VideoStream()
	if video == nil {
		return domain.ErrUnsupportedMedia
	}
	audio := probe.AudioStream()
	variant := &domain.Variant{
		MediaID:         asset.MediaID,
		Codec:           domain.CodecH264,
		Path:            path,
		Container:       uploadMP4Container,
		VideoCodec:      strings.ToLower(video.CodecName),
		Profile:         video.Profile,
		Level:           fmt.Sprintf("%d", video.Level),
		MIMEType:        uploadMP4MIME,
		Origin:          domain.VariantOriginClient,
		Primary:         true,
		Progress:        100,
		DurationSeconds: domain.ParseDuration(probe.Format.Duration),
		FileSize:        asset.ExpectedSize,
		Width:           video.Width,
		Height:          video.Height,
		Status:          domain.VariantStatusDone,
		CreatedAt:       now,
	}
	if audio != nil {
		variant.HasAudio = true
		variant.AudioCodec = strings.ToLower(audio.CodecName)
	}
	media, err := s.media.Get(asset.MediaID)
	if err != nil {
		return fmt.Errorf("load media for primary publication: %w", err)
	}
	media.MarkAsDone(path, domain.CodecH264, video.Width, video.Height, media.ThumbPath, asset.ExpectedSize)
	if reuseAsOriginal {
		if err := s.saveRetainedOriginal(asset, path, sha256sum, now); err != nil {
			return err
		}
	}
	if err := s.media.PublishPrimaryVariant(media, variant, probe.RawJSON); err != nil {
		if reuseAsOriginal {
			return s.rollbackRetainedOriginal(asset.MediaID, err)
		}
		return err
	}
	s.log.Infof("video upload path=direct media=%s codec=h264", asset.MediaID)
	return nil
}

func (s *UploadService) publishTransientSource(
	asset *domain.UploadAsset,
	path, sha256sum string,
	now time.Time,
	retainAsOriginal bool,
) error {
	if s.jobs == nil {
		return fmt.Errorf("%w: server fallback is unavailable", domain.ErrUnsupportedMedia)
	}
	sourceRole := domain.AssetRoleSourceTransient
	if retainAsOriginal {
		sourceRole = domain.AssetRoleOriginal
		if err := s.saveRetainedOriginal(asset, path, sha256sum, now); err != nil {
			return err
		}
	} else if err := s.media.SaveMediaAsset(availableMediaAsset(asset, sourceRole, path, sha256sum, now)); err != nil {
		return fmt.Errorf("save source-transient asset: %w", err)
	}
	if err := s.media.UpdateStatus(asset.MediaID, domain.MediaStatusProcessing, ""); err != nil {
		return s.rollbackFallbackSetup(asset.MediaID, sourceRole, fmt.Errorf("mark server fallback processing: %w", err))
	}
	variant, err := s.media.GetVariantByMediaAndCodec(asset.MediaID, domain.CodecH264)
	if errors.Is(err, domain.ErrNotFound) {
		variant = &domain.Variant{
			MediaID:   asset.MediaID,
			Codec:     domain.CodecH264,
			Origin:    domain.VariantOriginServer,
			Primary:   true,
			Progress:  0,
			Status:    domain.VariantStatusPending,
			CreatedAt: now,
		}
		if saveErr := s.media.SaveVariant(variant); saveErr != nil {
			return s.rollbackFallbackSetup(asset.MediaID, sourceRole, fmt.Errorf("save server fallback variant: %w", saveErr))
		}
	} else if err != nil {
		return s.rollbackFallbackSetup(asset.MediaID, sourceRole, fmt.Errorf("get server fallback variant: %w", err))
	}
	if variant.Status == domain.VariantStatusDone {
		return nil
	}
	if _, err := s.jobs.GetActive(asset.MediaID, domain.JobTypeConvert, domain.CodecH264); err == nil {
		return nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return s.rollbackFallbackSetup(asset.MediaID, sourceRole, fmt.Errorf("check server fallback conversion: %w", err))
	}
	if _, err := s.jobs.Enqueue(asset.MediaID, domain.JobTypeConvert, domain.CodecH264, serverFallbackFPS); err != nil {
		return s.rollbackFallbackSetup(asset.MediaID, sourceRole, fmt.Errorf("enqueue server fallback conversion: %w", err))
	}
	s.log.Infof("video upload path=server-fallback media=%s", asset.MediaID)
	return nil
}

func (s *UploadService) rollbackFallbackSetup(mediaID string, sourceRole domain.AssetRole, cause error) error {
	if err := s.media.DeleteMediaAsset(mediaID, sourceRole); err != nil {
		s.log.Warnf("rollback fallback source record media=%s err=%v", mediaID, err)
	}
	if sourceRole == domain.AssetRoleOriginal {
		if err := s.media.UpdateOriginalPath(mediaID, ""); err != nil {
			s.log.Warnf("rollback fallback original path media=%s err=%v", mediaID, err)
		}
	}
	if err := s.media.UpdateStatus(mediaID, domain.MediaStatusPending, ""); err != nil {
		s.log.Warnf("rollback fallback media status media=%s err=%v", mediaID, err)
	}
	return cause
}

func (s *UploadService) saveRetainedOriginal(asset *domain.UploadAsset, path, sha256sum string, now time.Time) error {
	if err := s.media.SaveMediaAsset(availableMediaAsset(asset, domain.AssetRoleOriginal, path, sha256sum, now)); err != nil {
		return fmt.Errorf("save retained original asset: %w", err)
	}
	if err := s.media.UpdateOriginalPath(asset.MediaID, path); err != nil {
		_ = s.media.DeleteMediaAsset(asset.MediaID, domain.AssetRoleOriginal)
		return fmt.Errorf("save retained original path: %w", err)
	}
	return nil
}

func (s *UploadService) rollbackRetainedOriginal(mediaID string, cause error) error {
	if err := s.media.DeleteMediaAsset(mediaID, domain.AssetRoleOriginal); err != nil {
		s.log.Warnf("rollback retained original media=%s err=%v", mediaID, err)
	}
	if err := s.media.UpdateOriginalPath(mediaID, ""); err != nil {
		s.log.Warnf("rollback retained original path media=%s err=%v", mediaID, err)
	}
	return cause
}

func sessionReusesPrimaryAsOriginal(session *domain.UploadSession) bool {
	if !session.KeepOriginal {
		return false
	}
	for i := range session.Assets {
		if session.Assets[i].Role == domain.AssetRoleOriginal {
			return false
		}
	}
	return true
}

func availableMediaAsset(
	upload *domain.UploadAsset,
	role domain.AssetRole,
	path string,
	sha256sum string,
	now time.Time,
) *domain.MediaAsset {
	return &domain.MediaAsset{
		MediaID:   upload.MediaID,
		Role:      role,
		Filename:  upload.Filename,
		Path:      path,
		Size:      upload.ExpectedSize,
		SHA256:    sha256sum,
		Status:    domain.AssetStatusAvailable,
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func (s *UploadService) resultForAvailable(session *domain.UploadSession, asset *domain.UploadAsset) (*FinalizeUploadResult, error) {
	media, err := s.media.Get(asset.MediaID)
	if err != nil {
		return nil, err
	}
	result := &FinalizeUploadResult{Session: session, Asset: asset, Media: media}
	for i := range media.Variants {
		if media.Variants[i].Codec == domain.CodecH264 && media.Variants[i].Primary {
			result.Variant = &media.Variants[i]
			break
		}
	}
	return result, nil
}

func newUploadAsset(
	session *domain.UploadSession,
	role domain.AssetRole,
	filename string,
	size int64,
	expectedSHA256 string,
	chunkSize int64,
	now time.Time,
) domain.UploadAsset {
	return domain.UploadAsset{
		ID:             newUploadID("asset"),
		SessionID:      session.ID,
		MediaID:        session.MediaID,
		Role:           role,
		Filename:       filename,
		ExpectedSize:   size,
		ChunkSize:      chunkSize,
		TotalChunks:    int((size + chunkSize - 1) / chunkSize),
		ExpectedSHA256: strings.ToLower(strings.TrimSpace(expectedSHA256)),
		Status:         domain.AssetStatusUploading,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

func validateChunkSet(asset *domain.UploadAsset, chunks []domain.UploadChunk) error {
	if len(chunks) != asset.TotalChunks {
		return domain.ErrUploadIncomplete
	}
	var total int64
	for i, chunk := range chunks {
		if chunk.Index != i || chunk.Size <= 0 {
			return domain.ErrUploadIncomplete
		}
		expected := min(asset.ExpectedSize-int64(i)*asset.ChunkSize, asset.ChunkSize)
		if chunk.Size != expected {
			return domain.ErrUploadIncomplete
		}
		total += chunk.Size
	}
	if total != asset.ExpectedSize {
		return domain.ErrUploadIncomplete
	}
	return nil
}

func normalizeUploadFilename(filename string) (string, error) {
	filename = strings.TrimSpace(filename)
	invalid := filename == "" || len(filename) > 255 || strings.ContainsRune(filename, '\x00')
	invalid = invalid || filepath.Base(filename) != filename || strings.ContainsAny(filename, "/\\")
	if invalid {
		return "", domain.ErrInvalidUpload
	}
	return filename, nil
}

func isPrimaryCompatible(probe *domain.ProbeResult, mime string, fastStart bool, formats []string) bool {
	video := probe.VideoStream()
	if video == nil || mime != uploadMP4MIME || !fastStart || !contains(formats, uploadMP4Container) {
		return false
	}
	if !strings.EqualFold(video.CodecName, "h264") || (video.PixFmt != "yuv420p" && video.PixFmt != "yuvj420p") {
		return false
	}
	fps := domain.ParseFrameRate(video.AvgFrameRate)
	if fps == 0 {
		fps = domain.ParseFrameRate(video.RFrameRate)
	}
	if fps > 60 || video.Level > 51 {
		return false
	}
	for i := range probe.Streams {
		stream := &probe.Streams[i]
		switch stream.CodecType {
		case probeVideoStream:
			if !strings.EqualFold(stream.CodecName, "h264") {
				return false
			}
		case "audio":
			if !strings.EqualFold(stream.CodecName, "aac") {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func allowedOriginalMIME(mime string) bool {
	switch mime {
	case "video/mp4", "video/quicktime", "video/webm", "audio/mpeg", "audio/ogg", "audio/wav", "audio/flac":
		return true
	default:
		return false
	}
}

func allowedOriginalFormat(formats []string) bool {
	for _, format := range formats {
		switch strings.TrimSpace(format) {
		case "mp4", "mov", uploadWebMFormat, "webm", "mpeg", "mp3", "ogg", "wav", "flac":
			return true
		}
	}
	return false
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == wanted {
			return true
		}
	}
	return false
}

func finalAssetName(asset *domain.UploadAsset, primaryReady bool) string {
	if asset.Role == domain.AssetRoleOriginal {
		return "original" + filepath.Ext(asset.Filename)
	}
	if primaryReady {
		return "primary.mp4"
	}
	return "source" + filepath.Ext(asset.Filename)
}

func newUploadID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	}
	return prefix + "-" + hex.EncodeToString(b[:])
}
