package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/bnema/sharm/internal/adapter/storage/sqlite/sqlitedb"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

type JobQueue struct {
	queries *sqlitedb.Queries
}

func NewJobQueue(store *Store) *JobQueue {
	return &JobQueue{
		queries: store.queries,
	}
}

func (q *JobQueue) Enqueue(mediaID string, jobType domain.JobType, codec domain.Codec, fps int) (*domain.Job, error) {
	ctx := context.Background()
	row, err := q.queries.InsertJob(ctx, sqlitedb.InsertJobParams{
		MediaID: mediaID,
		Type:    string(jobType),
		Codec:   string(codec),
		Fps:     int64(fps),
	})
	if err != nil {
		return nil, err
	}
	return jobFromRow(row), nil
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
	ctx := context.Background()
	return q.queries.CompleteJob(ctx, jobID)
}

func (q *JobQueue) Fail(jobID int64, errMsg string) error {
	ctx := context.Background()
	return q.queries.FailJob(ctx, sqlitedb.FailJobParams{
		ErrorMessage: errMsg,
		ID:           jobID,
	})
}

func (q *JobQueue) ResetStalled() error {
	ctx := context.Background()
	return q.queries.ResetStalledJobs(ctx)
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
		CreatedAt:    row.CreatedAt,
		StartedAt:    row.StartedAt,
		CompletedAt:  row.CompletedAt,
	}
}

var _ port.JobQueue = (*JobQueue)(nil)
