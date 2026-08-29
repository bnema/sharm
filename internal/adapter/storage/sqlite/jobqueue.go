package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/bnema/sharm/internal/adapter/storage/sqlite/sqlitedb"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

type JobQueue struct {
	db      *sql.DB
	queries *sqlitedb.Queries
}

func NewJobQueue(store *Store) *JobQueue {
	return &JobQueue{
		db:      store.db,
		queries: store.queries,
	}
}

func (q *JobQueue) Enqueue(mediaID string, jobType domain.JobType, codec domain.Codec, fps int) (*domain.Job, error) {
	ctx := context.Background()
	if existing, err := q.GetActive(mediaID, jobType, codec); err == nil {
		if existing.Fps != fps {
			return nil, domain.ErrJobConflict
		}
		return existing, nil
	} else if !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	row, err := q.queries.InsertJob(ctx, sqlitedb.InsertJobParams{
		MediaID: mediaID,
		Type:    string(jobType),
		Codec:   string(codec),
		Fps:     int64(fps),
	})
	if err == nil {
		return jobFromRow(row), nil
	}
	// A second process may have inserted the same active job between the
	// read and insert. The partial unique index makes this retry idempotent.
	if existing, getErr := q.GetActive(mediaID, jobType, codec); getErr == nil {
		if existing.Fps != fps {
			return nil, domain.ErrJobConflict
		}
		return existing, nil
	}
	return nil, err
}

func (q *JobQueue) GetActive(mediaID string, jobType domain.JobType, codec domain.Codec) (*domain.Job, error) {
	row, err := q.queries.GetActiveJob(context.Background(), sqlitedb.GetActiveJobParams{
		MediaID: mediaID,
		Type:    string(jobType),
		Codec:   string(codec),
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, domain.ErrNotFound
		}
		return nil, err
	}
	job := jobFromRow(row)
	return job, nil
}

func (q *JobQueue) Claim() (*domain.Job, error) {
	ctx := context.Background()
	row, err := q.queries.ClaimNextJob(ctx)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return jobFromRow(row), nil
}

func (q *JobQueue) Complete(jobID int64) error {
	rows, err := q.queries.CompleteJob(context.Background(), jobID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (q *JobQueue) UpdateProgress(jobID int64, progress int) error {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	rows, err := q.queries.UpdateJobProgress(context.Background(), sqlitedb.UpdateJobProgressParams{
		Progress: int64(progress),
		ID:       jobID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (q *JobQueue) Heartbeat(jobID int64) error {
	rows, err := q.queries.HeartbeatJob(context.Background(), jobID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (q *JobQueue) Fail(jobID int64, errMsg string) error {
	rows, err := q.queries.FailJob(context.Background(), sqlitedb.FailJobParams{
		ErrorMessage: errMsg,
		ID:           jobID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (q *JobQueue) ResetStalled() ([]domain.Job, error) {
	ctx := context.Background()
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	queries := q.queries.WithTx(tx)
	rows, err := queries.ListExhaustedStalledJobs(ctx)
	if err != nil {
		return nil, err
	}
	if err := queries.ResetStalledJobs(ctx); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	exhausted := make([]domain.Job, 0, len(rows))
	for i := range rows {
		job := jobFromRow(rows[i])
		job.Status = domain.JobStatusFailed
		job.ErrorMessage = "job exceeded retry limit"
		exhausted = append(exhausted, *job)
	}
	return exhausted, nil
}

func jobFromRow(row sqlitedb.Job) *domain.Job {
	return &domain.Job{
		ID:           row.ID,
		MediaID:      row.MediaID,
		Type:         domain.JobType(row.Type),
		Codec:        domain.Codec(row.Codec),
		Fps:          int(row.Fps),
		Status:       domain.JobStatus(row.Status),
		ErrorMessage: row.ErrorMessage,
		Attempts:     row.Attempts,
		Progress:     int(row.Progress),
		MaxAttempts:  row.MaxAttempts,
		CreatedAt:    row.CreatedAt,
		StartedAt:    nullableTime(row.StartedAt),
		CompletedAt:  nullableTime(row.CompletedAt),
		LeaseUntil:   nullableTime(row.LeaseUntil),
		HeartbeatAt:  nullableTime(row.HeartbeatAt),
	}
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}

var _ port.JobQueue = (*JobQueue)(nil)
