package domain

import (
	"database/sql"
	"time"
)

type JobType string

const (
	JobTypeConvert   JobType = "convert"
	JobTypeThumbnail JobType = "thumbnail"
	JobTypeProbe     JobType = "probe"
)

type JobStatus string

const (
	JobStatusPending JobStatus = "pending"
	JobStatusRunning JobStatus = "running"
	JobStatusDone    JobStatus = "done"
	JobStatusFailed  JobStatus = "failed"
)

type Job struct {
	ID           int64
	MediaID      string
	Type         JobType
	Codec        Codec
	Fps          int
	Status       JobStatus
	ErrorMessage string
	Attempts     int64
	CreatedAt    time.Time
	StartedAt    sql.NullTime
	CompletedAt  sql.NullTime
}
