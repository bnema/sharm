package port

import "github.com/bnema/sharm/internal/domain"

type JobQueue interface {
	Enqueue(mediaID string, jobType domain.JobType, codec domain.Codec, fps int) (*domain.Job, error)
	Claim() (*domain.Job, error)
	Complete(jobID int64) error
	Fail(jobID int64, errMsg string) error
	ResetStalled() error
}
