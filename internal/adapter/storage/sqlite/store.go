package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sync"

	"github.com/bnema/sharm/internal/adapter/storage/sqlite/sqlitedb"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
	"github.com/pressly/goose/v3"
	"modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct {
	db      *sql.DB
	queries *sqlitedb.Queries
}

var hookOnce sync.Once

func registerHook() {
	hookOnce.Do(func() {
		sqlite.RegisterConnectionHook(func(conn sqlite.ExecQuerierContext, dsn string) error {
			pragmas := []string{
				"PRAGMA journal_mode = WAL",
				"PRAGMA busy_timeout = 5000",
				"PRAGMA synchronous = NORMAL",
				"PRAGMA foreign_keys = ON",
				"PRAGMA cache_size = -8000",    // 8MB
				"PRAGMA mmap_size = 268435456", // 256MB
			}
			for _, p := range pragmas {
				if _, err := conn.ExecContext(context.Background(), p, nil); err != nil {
					return fmt.Errorf("execute %s: %w", p, err)
				}
			}
			return nil
		})
	})
}

func NewStore(dataDir string) (*Store, error) {
	registerHook()

	dbPath := dataDir + "/sharm.db"
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Single connection for SQLite (WAL allows concurrent reads but only one writer)
	db.SetMaxOpenConns(1)

	// Run migrations
	goose.SetBaseFS(migrations)
	if err := goose.SetDialect("sqlite3"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.Up(db, "migrations"); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Store{
		db:      db,
		queries: sqlitedb.New(db),
	}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

func (s *Store) Queries() *sqlitedb.Queries {
	return s.queries
}

func (s *Store) Save(m *domain.Media) error {
	ctx := context.Background()
	return s.queries.InsertMedia(ctx, sqlitedb.InsertMediaParams{
		ID:            m.ID,
		Type:          string(m.Type),
		OriginalName:  m.OriginalName,
		OriginalPath:  m.OriginalPath,
		ConvertedPath: m.ConvertedPath,
		Status:        string(m.Status),
		Codec:         string(m.Codec),
		ErrorMessage:  m.ErrorMessage,
		RetentionDays: int64(m.RetentionDays),
		FileSize:      m.FileSize,
		Width:         int64(m.Width),
		Height:        int64(m.Height),
		ThumbPath:     m.ThumbPath,
		CreatedAt:     m.CreatedAt,
		ExpiresAt:     m.ExpiresAt,
	})
}

func (s *Store) Get(id string) (*domain.Media, error) {
	ctx := context.Background()
	row, err := s.queries.GetMedia(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	media := mediumToMedia(row)

	// Load variants
	variants, err := s.queries.ListVariantsByMedia(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("list variants: %w", err)
	}
	media.Variants = variantListFromRows(variants)

	return media, nil
}

func (s *Store) Delete(id string) error {
	ctx := context.Background()
	return s.queries.DeleteMedia(ctx, id)
}

func (s *Store) ListExpired() ([]*domain.Media, error) {
	ctx := context.Background()
	rows, err := s.queries.ListExpiredMedia(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaListWithVariants(ctx, rows)
}

func (s *Store) ListAll() ([]*domain.Media, error) {
	ctx := context.Background()
	rows, err := s.queries.ListAllMedia(ctx)
	if err != nil {
		return nil, err
	}
	return s.mediaListWithVariants(ctx, rows)
}

func (s *Store) UpdateStatus(id string, status domain.MediaStatus, errMsg string) error {
	ctx := context.Background()
	return s.queries.UpdateMediaStatus(ctx, sqlitedb.UpdateMediaStatusParams{
		Status:       string(status),
		ErrorMessage: errMsg,
		ID:           id,
	})
}

func (s *Store) UpdateDone(m *domain.Media) error {
	ctx := context.Background()
	return s.queries.UpdateMediaDone(ctx, sqlitedb.UpdateMediaDoneParams{
		ConvertedPath: m.ConvertedPath,
		Codec:         string(m.Codec),
		Width:         int64(m.Width),
		Height:        int64(m.Height),
		ThumbPath:     m.ThumbPath,
		FileSize:      m.FileSize,
		ID:            m.ID,
	})
}

// Variant methods

func (s *Store) SaveVariant(v *domain.Variant) error {
	ctx := context.Background()
	row, err := s.queries.InsertVariant(ctx, sqlitedb.InsertVariantParams{
		MediaID: v.MediaID,
		Codec:   string(v.Codec),
	})
	if err != nil {
		return err
	}
	v.ID = row.ID
	v.CreatedAt = row.CreatedAt
	return nil
}

func (s *Store) GetVariant(id int64) (*domain.Variant, error) {
	ctx := context.Background()
	row, err := s.queries.GetVariant(ctx, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	v := variantFromRow(row)
	return &v, nil
}

func (s *Store) GetVariantByMediaAndCodec(mediaID string, codec domain.Codec) (*domain.Variant, error) {
	ctx := context.Background()
	row, err := s.queries.GetVariantByMediaAndCodec(ctx, sqlitedb.GetVariantByMediaAndCodecParams{
		MediaID: mediaID,
		Codec:   string(codec),
	})
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	v := variantFromRow(row)
	return &v, nil
}

func (s *Store) ListVariantsByMedia(mediaID string) ([]domain.Variant, error) {
	ctx := context.Background()
	rows, err := s.queries.ListVariantsByMedia(ctx, mediaID)
	if err != nil {
		return nil, err
	}
	return variantListFromRows(rows), nil
}

func (s *Store) UpdateVariantStatus(id int64, status domain.VariantStatus, errMsg string) error {
	ctx := context.Background()
	return s.queries.UpdateVariantStatus(ctx, sqlitedb.UpdateVariantStatusParams{
		Status:       string(status),
		ErrorMessage: errMsg,
		ID:           id,
	})
}

func (s *Store) UpdateVariantDone(v *domain.Variant) error {
	ctx := context.Background()
	return s.queries.UpdateVariantDone(ctx, sqlitedb.UpdateVariantDoneParams{
		Path:     v.Path,
		FileSize: v.FileSize,
		Width:    int64(v.Width),
		Height:   int64(v.Height),
		ID:       v.ID,
	})
}

func (s *Store) DeleteVariantsByMedia(mediaID string) error {
	ctx := context.Background()
	return s.queries.DeleteVariantsByMedia(ctx, mediaID)
}

// Helper conversions

func mediumToMedia(row sqlitedb.Medium) *domain.Media {
	return &domain.Media{
		ID:            row.ID,
		Type:          domain.MediaType(row.Type),
		OriginalName:  row.OriginalName,
		OriginalPath:  row.OriginalPath,
		ConvertedPath: row.ConvertedPath,
		Status:        domain.MediaStatus(row.Status),
		Codec:         domain.Codec(row.Codec),
		ErrorMessage:  row.ErrorMessage,
		RetentionDays: int(row.RetentionDays),
		FileSize:      row.FileSize,
		Width:         int(row.Width),
		Height:        int(row.Height),
		ThumbPath:     row.ThumbPath,
		CreatedAt:     row.CreatedAt,
		ExpiresAt:     row.ExpiresAt,
	}
}

func variantFromRow(row sqlitedb.MediaVariant) domain.Variant {
	return domain.Variant{
		ID:           row.ID,
		MediaID:      row.MediaID,
		Codec:        domain.Codec(row.Codec),
		Path:         row.Path,
		FileSize:     row.FileSize,
		Width:        int(row.Width),
		Height:       int(row.Height),
		Status:       domain.VariantStatus(row.Status),
		ErrorMessage: row.ErrorMessage,
		CreatedAt:    row.CreatedAt,
	}
}

func variantListFromRows(rows []sqlitedb.MediaVariant) []domain.Variant {
	result := make([]domain.Variant, len(rows))
	for i, row := range rows {
		result[i] = variantFromRow(row)
	}
	return result
}

func (s *Store) mediaListWithVariants(ctx context.Context, rows []sqlitedb.Medium) ([]*domain.Media, error) {
	result := make([]*domain.Media, len(rows))
	for i, row := range rows {
		media := mediumToMedia(row)
		variants, err := s.queries.ListVariantsByMedia(ctx, media.ID)
		if err != nil {
			return nil, fmt.Errorf("list variants for %s: %w", media.ID, err)
		}
		media.Variants = variantListFromRows(variants)
		result[i] = media
	}
	return result, nil
}

var _ port.MediaStore = (*Store)(nil)
