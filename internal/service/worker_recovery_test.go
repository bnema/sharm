package service

import (
	"errors"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
	"github.com/bnema/sharm/internal/port/mocks"
)

func TestWorkerPoolResetStalledJobsSkipsEventWhenLegacyStatusUpdateFails(t *testing.T) {
	job := domain.Job{
		MediaID:      "media-1",
		Type:         domain.JobTypeConvert,
		ErrorMessage: "conversion attempts exhausted",
	}
	queue := mocks.NewJobQueueMock(t)
	queue.EXPECT().ResetStalled().Return([]domain.Job{job}, nil)
	store := mocks.NewMediaStoreMock(t)
	store.EXPECT().UpdateStatus(job.MediaID, domain.MediaStatusFailed, job.ErrorMessage).Return(errors.New("update status"))

	worker := &WorkerPool{
		jobQueue: queue,
		store:    store,
		eventBus: mocks.NewEventPublisherMock(t),
		log:      nopLogger{},
	}

	worker.resetStalledJobs()
}

func TestWorkerPoolResetStalledJobsPublishesPersistedLegacyFailure(t *testing.T) {
	job := domain.Job{
		MediaID:      "media-1",
		Type:         domain.JobTypeConvert,
		ErrorMessage: "conversion attempts exhausted",
	}
	queue := mocks.NewJobQueueMock(t)
	queue.EXPECT().ResetStalled().Return([]domain.Job{job}, nil)
	store := mocks.NewMediaStoreMock(t)
	store.EXPECT().UpdateStatus(job.MediaID, domain.MediaStatusFailed, job.ErrorMessage).Return(nil)
	events := mocks.NewEventPublisherMock(t)
	events.EXPECT().Publish(job.MediaID, port.Event{
		Type:    "status",
		Status:  string(domain.MediaStatusFailed),
		Message: job.ErrorMessage,
	}).Return()

	worker := &WorkerPool{
		jobQueue: queue,
		store:    store,
		eventBus: events,
		log:      nopLogger{},
	}

	worker.resetStalledJobs()
}
