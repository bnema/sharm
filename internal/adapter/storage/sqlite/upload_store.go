package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/bnema/sharm/internal/adapter/storage/sqlite/sqlitedb"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

func (s *Store) CreateUploadSession(session *domain.UploadSession, assets []domain.UploadAsset, maxReservedBytes int64) error {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin upload session transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	q := s.queries.WithTx(tx)
	rows, err := q.InsertUploadSession(ctx, sqlitedb.InsertUploadSessionParams{
		ID:               session.ID,
		MediaID:          session.MediaID,
		UserID:           session.UserID,
		Filename:         session.Filename,
		RetentionDays:    int64(session.RetentionDays),
		KeepOriginal:     boolToInt64(session.KeepOriginal),
		ExpectedBytes:    session.ExpectedBytes,
		ReservedBytes:    session.ReservedBytes,
		Status:           string(session.Status),
		ExpiresAt:        session.ExpiresAt,
		CreatedAt:        session.CreatedAt,
		UpdatedAt:        session.UpdatedAt,
		MaxReservedBytes: maxReservedBytes,
	})
	if err != nil {
		return fmt.Errorf("insert upload session: %w", err)
	}
	if rows == 0 {
		return domain.ErrQuotaExceeded
	}
	for i := range assets {
		asset := &assets[i]
		if err := q.InsertUploadAsset(ctx, sqlitedb.InsertUploadAssetParams{
			ID:             asset.ID,
			SessionID:      asset.SessionID,
			MediaID:        asset.MediaID,
			Role:           string(asset.Role),
			Filename:       asset.Filename,
			ExpectedSize:   asset.ExpectedSize,
			ChunkSize:      asset.ChunkSize,
			TotalChunks:    int64(asset.TotalChunks),
			ExpectedSha256: asset.ExpectedSHA256,
			Status:         string(asset.Status),
			CreatedAt:      asset.CreatedAt,
			UpdatedAt:      asset.UpdatedAt,
		}); err != nil {
			return fmt.Errorf("insert upload asset %s: %w", asset.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit upload session: %w", err)
	}
	return nil
}

func (s *Store) GetUploadSession(id string) (*domain.UploadSession, error) {
	ctx := context.Background()
	row, err := s.queries.GetUploadSession(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	session := uploadSessionFromRow(row)
	assets, err := s.queries.GetUploadAssetsBySession(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list upload assets: %w", err)
	}
	session.Assets = make([]domain.UploadAsset, 0, len(assets))
	for i := range assets {
		asset := uploadAssetFromRow(assets[i])
		chunks, err := s.queries.ListUploadChunks(ctx, asset.ID)
		if err != nil {
			return nil, fmt.Errorf("list upload chunks for asset %s: %w", asset.ID, err)
		}
		asset.Chunks = make([]domain.UploadChunk, 0, len(chunks))
		for _, chunkRow := range chunks {
			asset.Chunks = append(asset.Chunks, uploadChunkFromRow(chunkRow))
		}
		session.Assets = append(session.Assets, asset)
	}
	return &session, nil
}

func (s *Store) GetUploadAsset(id string) (*domain.UploadAsset, error) {
	ctx := context.Background()
	row, err := s.queries.GetUploadAsset(ctx, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	asset := uploadAssetFromRow(row)
	chunks, err := s.queries.ListUploadChunks(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list upload chunks: %w", err)
	}
	asset.Chunks = make([]domain.UploadChunk, 0, len(chunks))
	for _, chunk := range chunks {
		asset.Chunks = append(asset.Chunks, uploadChunkFromRow(chunk))
	}
	return &asset, nil
}

func (s *Store) GetUploadAssetByRole(sessionID string, role domain.AssetRole) (*domain.UploadAsset, error) {
	ctx := context.Background()
	row, err := s.queries.GetUploadAssetBySessionAndRole(ctx, sqlitedb.GetUploadAssetBySessionAndRoleParams{
		SessionID: sessionID,
		Role:      string(role),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	asset := uploadAssetFromRow(row)
	return &asset, nil
}

func (s *Store) ListUploadChunks(assetID string) ([]domain.UploadChunk, error) {
	rows, err := s.queries.ListUploadChunks(context.Background(), assetID)
	if err != nil {
		return nil, err
	}
	result := make([]domain.UploadChunk, len(rows))
	for i, row := range rows {
		result[i] = uploadChunkFromRow(row)
	}
	return result, nil
}

func (s *Store) RecordUploadChunk(assetID string, chunk *domain.UploadChunk) (bool, error) {
	ctx := context.Background()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin chunk transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	q := s.queries.WithTx(tx)

	asset, err := q.GetUploadAsset(ctx, assetID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, domain.ErrNotFound
		}
		return false, err
	}
	if asset.Status != string(domain.AssetStatusUploading) {
		return false, domain.ErrInvalidUpload
	}

	existing, err := q.GetUploadChunk(ctx, sqlitedb.GetUploadChunkParams{
		AssetID:    assetID,
		ChunkIndex: int64(chunk.Index),
	})
	if err == nil {
		if existing.SizeBytes != chunk.Size || existing.Sha256 != chunk.SHA256 {
			return false, domain.ErrChunkConflict
		}
		return false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}

	rows, err := q.InsertUploadChunk(ctx, sqlitedb.InsertUploadChunkParams{
		AssetID:    assetID,
		ChunkIndex: int64(chunk.Index),
		SizeBytes:  chunk.Size,
		Sha256:     chunk.SHA256,
		CreatedAt:  chunk.CreatedAt,
	})
	if err != nil {
		return false, err
	}
	if rows == 0 {
		existing, getErr := q.GetUploadChunk(ctx, sqlitedb.GetUploadChunkParams{
			AssetID: assetID, ChunkIndex: int64(chunk.Index),
		})
		if getErr != nil {
			return false, getErr
		}
		if existing.SizeBytes != chunk.Size || existing.Sha256 != chunk.SHA256 {
			return false, domain.ErrChunkConflict
		}
		return false, nil
	}
	if err := q.IncrementUploadAssetReceived(ctx, sqlitedb.IncrementUploadAssetReceivedParams{
		ReceivedBytes: chunk.Size,
		UpdatedAt:     chunk.CreatedAt,
		ID:            assetID,
	}); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit chunk metadata: %w", err)
	}
	return true, nil
}

func (s *Store) ClaimUploadAssetFinalization(id string, now time.Time) (bool, error) {
	rows, err := s.queries.ClaimUploadAssetFinalization(context.Background(), sqlitedb.ClaimUploadAssetFinalizationParams{
		UpdatedAt: now,
		ID:        id,
	})
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

func (s *Store) ReleaseUploadAssetFinalization(id, errMsg string, now time.Time) error {
	return s.queries.ReleaseUploadAssetFinalization(context.Background(), sqlitedb.ReleaseUploadAssetFinalizationParams{
		ErrorMessage: errMsg,
		UpdatedAt:    now,
		ID:           id,
	})
}

func (s *Store) CompleteUploadAsset(id, path, sha256 string, receivedBytes int64, completedAt time.Time) error {
	rows, err := s.queries.CompleteUploadAsset(context.Background(), sqlitedb.CompleteUploadAssetParams{
		ReceivedBytes: receivedBytes,
		Sha256:        sha256,
		Path:          path,
		UpdatedAt:     completedAt,
		CompletedAt:   sql.NullTime{Time: completedAt, Valid: true},
		ID:            id,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) FailUploadAsset(id, errMsg string, now time.Time) error {
	rows, err := s.queries.FailUploadAsset(context.Background(), sqlitedb.FailUploadAssetParams{
		ErrorMessage: errMsg,
		UpdatedAt:    now,
		ID:           id,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (s *Store) UpdateUploadSessionStatus(id string, status domain.UploadSessionStatus, now time.Time) error {
	return s.queries.UpdateUploadSessionStatus(context.Background(), sqlitedb.UpdateUploadSessionStatusParams{
		Status:    string(status),
		UpdatedAt: now,
		ID:        id,
	})
}

func (s *Store) ListExpiredUploadSessions(now time.Time) ([]domain.UploadSession, error) {
	rows, err := s.queries.ListExpiredUploadSessions(context.Background(), now)
	if err != nil {
		return nil, err
	}
	result := make([]domain.UploadSession, len(rows))
	for i := range rows {
		result[i] = uploadSessionFromRow(rows[i])
	}
	return result, nil
}

func (s *Store) DeleteUploadSession(id string) error {
	return s.queries.DeleteUploadSession(context.Background(), id)
}

func (s *Store) GetMediaAsset(mediaID string, role domain.AssetRole) (*domain.MediaAsset, error) {
	row, err := s.queries.GetMediaAssetByMediaAndRole(context.Background(), sqlitedb.GetMediaAssetByMediaAndRoleParams{
		MediaID: mediaID,
		Role:    string(role),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	asset := mediaAssetFromRow(row)
	return &asset, nil
}

func (s *Store) SaveMediaAsset(asset *domain.MediaAsset) error {
	if asset.CreatedAt.IsZero() {
		asset.CreatedAt = time.Now()
	}
	if asset.UpdatedAt.IsZero() {
		asset.UpdatedAt = asset.CreatedAt
	}
	if asset.ID == "" {
		asset.ID = newAssetID(asset.MediaID, asset.Role, asset.CreatedAt)
	}
	return s.queries.UpsertMediaAsset(context.Background(), sqlitedb.UpsertMediaAssetParams{
		ID:           asset.ID,
		MediaID:      asset.MediaID,
		Role:         string(asset.Role),
		Filename:     asset.Filename,
		Path:         asset.Path,
		SizeBytes:    asset.Size,
		Sha256:       asset.SHA256,
		Status:       string(asset.Status),
		ErrorMessage: asset.ErrorMessage,
		CreatedAt:    asset.CreatedAt,
		UpdatedAt:    asset.UpdatedAt,
	})
}

func (s *Store) DeleteMediaAsset(mediaID string, role domain.AssetRole) error {
	return s.queries.DeleteMediaAssetByRole(context.Background(), sqlitedb.DeleteMediaAssetByRoleParams{
		MediaID: mediaID,
		Role:    string(role),
	})
}

func (s *Store) DeleteMediaAssets(mediaID string) error {
	return s.queries.DeleteMediaAssetsByMedia(context.Background(), mediaID)
}

func uploadSessionFromRow(row sqlitedb.UploadSession) domain.UploadSession {
	return domain.UploadSession{
		ID:            row.ID,
		MediaID:       row.MediaID,
		UserID:        row.UserID,
		Filename:      row.Filename,
		RetentionDays: int(row.RetentionDays),
		KeepOriginal:  row.KeepOriginal != 0,
		ExpectedBytes: row.ExpectedBytes,
		ReservedBytes: row.ReservedBytes,
		Status:        domain.UploadSessionStatus(row.Status),
		ExpiresAt:     row.ExpiresAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
	}
}

func uploadAssetFromRow(row sqlitedb.UploadAsset) domain.UploadAsset {
	asset := domain.UploadAsset{
		ID:             row.ID,
		SessionID:      row.SessionID,
		MediaID:        row.MediaID,
		Role:           domain.AssetRole(row.Role),
		Filename:       row.Filename,
		ExpectedSize:   row.ExpectedSize,
		ChunkSize:      row.ChunkSize,
		TotalChunks:    int(row.TotalChunks),
		ReceivedBytes:  row.ReceivedBytes,
		ExpectedSHA256: row.ExpectedSha256,
		SHA256:         row.Sha256,
		Status:         domain.AssetStatus(row.Status),
		Path:           row.Path,
		ErrorMessage:   row.ErrorMessage,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
	}
	if row.CompletedAt.Valid {
		completedAt := row.CompletedAt.Time
		asset.CompletedAt = &completedAt
	}
	return asset
}

func uploadChunkFromRow(row sqlitedb.UploadChunk) domain.UploadChunk {
	return domain.UploadChunk{
		AssetID:   row.AssetID,
		Index:     int(row.ChunkIndex),
		Size:      row.SizeBytes,
		SHA256:    row.Sha256,
		CreatedAt: row.CreatedAt,
	}
}

func mediaAssetFromRow(row sqlitedb.MediaAsset) domain.MediaAsset {
	return domain.MediaAsset{
		ID:           row.ID,
		MediaID:      row.MediaID,
		Role:         domain.AssetRole(row.Role),
		Filename:     row.Filename,
		Path:         row.Path,
		Size:         row.SizeBytes,
		SHA256:       row.Sha256,
		Status:       domain.AssetStatus(row.Status),
		ErrorMessage: row.ErrorMessage,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

func newAssetID(mediaID string, role domain.AssetRole, now time.Time) string {
	return fmt.Sprintf("asset-%s-%s-%d", mediaID, role, now.UnixNano())
}

var _ port.UploadStore = (*Store)(nil)
