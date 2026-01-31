package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/infrastructure/logger"
	"github.com/bnema/sharm/internal/port"
)

type WorkerPool struct {
	jobQueue  port.JobQueue
	store     port.MediaStore
	converter port.MediaConverter
	eventBus  EventPublisher
	dataDir   string
	workers   int
}

type EventPublisher interface {
	Publish(mediaID string, event Event)
}

type Event struct {
	Type    string // "status", "progress"
	Status  string
	Message string
}

func NewWorkerPool(
	jobQueue port.JobQueue,
	store port.MediaStore,
	converter port.MediaConverter,
	eventBus EventPublisher,
	dataDir string,
	workers int,
) *WorkerPool {
	return &WorkerPool{
		jobQueue:  jobQueue,
		store:     store,
		converter: converter,
		eventBus:  eventBus,
		dataDir:   dataDir,
		workers:   workers,
	}
}

func (wp *WorkerPool) Start(ctx context.Context) {
	// Reset any stalled jobs from previous runs
	if err := wp.jobQueue.ResetStalled(); err != nil {
		logger.Error.Printf("failed to reset stalled jobs: %v", err)
	}

	for i := range wp.workers {
		go wp.runWorker(ctx, i)
	}
	logger.Info.Printf("started %d workers", wp.workers)
}

func (wp *WorkerPool) runWorker(ctx context.Context, id int) {
	for {
		select {
		case <-ctx.Done():
			logger.Info.Printf("worker %d shutting down", id)
			return
		default:
		}

		job, err := wp.jobQueue.Claim()
		if err != nil {
			logger.Error.Printf("worker %d: failed to claim job: %v", id, err)
			time.Sleep(2 * time.Second)
			continue
		}

		if job == nil {
			// No pending jobs, wait before polling again
			time.Sleep(500 * time.Millisecond)
			continue
		}

		logger.Info.Printf("worker %d: processing job %d (type=%s, media=%s, codec=%s)", id, job.ID, job.Type, job.MediaID, job.Codec)
		wp.processJob(job)
	}
}

func (wp *WorkerPool) processJob(job *domain.Job) {
	var err error

	switch job.Type {
	case domain.JobTypeConvert:
		err = wp.handleConvert(job)
	case domain.JobTypeThumbnail:
		err = wp.handleThumbnail(job)
	case domain.JobTypeProbe:
		err = wp.handleProbe(job)
	default:
		err = fmt.Errorf("unknown job type: %s", job.Type)
	}

	if err != nil {
		logger.Error.Printf("job %d failed: %v", job.ID, err)
		_ = wp.jobQueue.Fail(job.ID, err.Error())

		// If this was a convert job with a codec, mark the variant as failed
		if job.Type == domain.JobTypeConvert && job.Codec != "" {
			wp.failVariant(job)
		} else if job.Type == domain.JobTypeConvert {
			// Legacy: no codec means old-style conversion
			_ = wp.store.UpdateStatus(job.MediaID, domain.MediaStatusFailed, err.Error())
			wp.publishEvent(job.MediaID, "status", string(domain.MediaStatusFailed), err.Error())
		}
		return
	}

	_ = wp.jobQueue.Complete(job.ID)
	logger.Info.Printf("job %d completed", job.ID)
}

func (wp *WorkerPool) handleConvert(job *domain.Job) error {
	media, err := wp.store.Get(job.MediaID)
	if err != nil {
		return fmt.Errorf("get media: %w", err)
	}

	// Update media status to processing (if not already)
	if media.Status == domain.MediaStatusPending {
		_ = wp.store.UpdateStatus(media.ID, domain.MediaStatusProcessing, "")
		wp.publishEvent(media.ID, "status", string(domain.MediaStatusProcessing), "")
	}

	convertedDir := filepath.Join(wp.dataDir, "converted")
	if err := os.MkdirAll(convertedDir, 0755); err != nil {
		return fmt.Errorf("create converted directory: %w", err)
	}

	// Per-variant conversion
	if job.Codec != "" {
		return wp.handleVariantConvert(job, media, convertedDir)
	}

	// Legacy: old-style conversion (no codec specified, try AV1 then H264)
	return wp.handleLegacyConvert(job, media, convertedDir)
}

func (wp *WorkerPool) handleVariantConvert(job *domain.Job, media *domain.Media, convertedDir string) error {
	variant, err := wp.store.GetVariantByMediaAndCodec(media.ID, job.Codec)
	if err != nil {
		return fmt.Errorf("get variant: %w", err)
	}

	_ = wp.store.UpdateVariantStatus(variant.ID, domain.VariantStatusProcessing, "")
	wp.publishEvent(media.ID, "status", string(domain.MediaStatusProcessing), "")

	outputPath, err := wp.converter.ConvertCodec(media.OriginalPath, convertedDir, media.ID, job.Codec, job.Fps)
	if err != nil {
		return fmt.Errorf("convert %s: %w", job.Codec, err)
	}

	var width, height int
	var probeJSON string
	if media.Type == domain.MediaTypeVideo {
		probeResult, probeErr := wp.converter.Probe(outputPath)
		if probeErr != nil {
			logger.Error.Printf("probe failed for variant %s: %v", job.Codec, probeErr)
		} else {
			width, height = probeResult.Dimensions()
			probeJSON = probeResult.RawJSON
			if media.ProbeJSON == "" {
				_ = wp.store.UpdateProbeJSON(media.ID, probeJSON)
			}
		}
	}

	fileInfo, _ := os.Stat(outputPath)
	var fileSize int64
	if fileInfo != nil {
		fileSize = fileInfo.Size()
	}

	variant.Path = outputPath
	variant.FileSize = fileSize
	variant.Width = width
	variant.Height = height
	variant.Status = domain.VariantStatusDone
	if err := wp.store.UpdateVariantDone(variant); err != nil {
		return fmt.Errorf("update variant done: %w", err)
	}

	if media.Type == domain.MediaTypeVideo && media.ThumbPath == "" {
		thumbPath := filepath.Join(convertedDir, media.ID+"_thumb.jpg")
		if err := wp.converter.Thumbnail(outputPath, thumbPath); err != nil {
			logger.Error.Printf("thumbnail failed for %s: %v", media.ID, err)
		} else {
			media.ThumbPath = thumbPath
		}
	}

	media, err = wp.store.Get(media.ID)
	if err != nil {
		return fmt.Errorf("re-fetch media: %w", err)
	}

	if media.AllVariantsTerminal() {
		best := media.BestVariant()
		if best != nil {
			media.MarkAsDone(best.Path, best.Codec, best.Width, best.Height, media.ThumbPath, best.FileSize)
		} else {
			media.Status = domain.MediaStatusFailed
			media.ErrorMessage = "all conversions failed"
			_ = wp.store.UpdateStatus(media.ID, domain.MediaStatusFailed, "all conversions failed")
			wp.publishEvent(media.ID, "status", string(domain.MediaStatusFailed), "all conversions failed")
			return nil
		}
		if err := wp.store.UpdateDone(media); err != nil {
			return fmt.Errorf("update media done: %w", err)
		}
		wp.publishEvent(media.ID, "status", string(domain.MediaStatusDone), "")
	} else {
		wp.publishEvent(media.ID, "status", string(domain.MediaStatusProcessing), "")
	}

	return nil
}

func (wp *WorkerPool) handleLegacyConvert(job *domain.Job, media *domain.Media, convertedDir string) error {
	convertedPath, codec, err := wp.converter.Convert(media.OriginalPath, convertedDir, media.ID)
	if err != nil {
		return fmt.Errorf("convert: %w", err)
	}

	probeResult, err := wp.converter.Probe(convertedPath)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}
	width, height := probeResult.Dimensions()

	thumbPath := filepath.Join(convertedDir, media.ID+"_thumb.jpg")
	if err := wp.converter.Thumbnail(convertedPath, thumbPath); err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}

	fileInfo, _ := os.Stat(convertedPath)
	media.MarkAsDone(convertedPath, domain.Codec(codec), width, height, thumbPath, fileInfo.Size())

	if err := wp.store.UpdateDone(media); err != nil {
		return fmt.Errorf("update media done: %w", err)
	}

	_ = os.Remove(media.OriginalPath)

	wp.publishEvent(media.ID, "status", string(domain.MediaStatusDone), "")
	return nil
}

func (wp *WorkerPool) failVariant(job *domain.Job) {
	variant, err := wp.store.GetVariantByMediaAndCodec(job.MediaID, job.Codec)
	if err != nil {
		logger.Error.Printf("failed to get variant for failure update: %v", err)
		return
	}
	_ = wp.store.UpdateVariantStatus(variant.ID, domain.VariantStatusFailed, job.ErrorMessage)

	// Re-fetch media to check if all variants are terminal
	media, err := wp.store.Get(job.MediaID)
	if err != nil {
		logger.Error.Printf("failed to re-fetch media after variant failure: %v", err)
		return
	}

	if media.AllVariantsTerminal() {
		if media.HasDoneVariant() {
			best := media.BestVariant()
			media.MarkAsDone(best.Path, best.Codec, best.Width, best.Height, media.ThumbPath, best.FileSize)
			if err := wp.store.UpdateDone(media); err != nil {
				logger.Error.Printf("failed to mark media done after variant failures: %v", err)
			}
			wp.publishEvent(media.ID, "status", string(domain.MediaStatusDone), "")
		} else {
			_ = wp.store.UpdateStatus(media.ID, domain.MediaStatusFailed, "all conversions failed")
			wp.publishEvent(media.ID, "status", string(domain.MediaStatusFailed), "all conversions failed")
		}
	}
}

func (wp *WorkerPool) handleThumbnail(job *domain.Job) error {
	media, err := wp.store.Get(job.MediaID)
	if err != nil {
		return fmt.Errorf("get media: %w", err)
	}

	convertedDir := filepath.Join(wp.dataDir, "converted")
	if err := os.MkdirAll(convertedDir, 0755); err != nil {
		return fmt.Errorf("create converted directory: %w", err)
	}
	thumbPath := filepath.Join(convertedDir, media.ID+"_thumb.jpg")

	// Use original path as source for thumbnail
	sourcePath := media.OriginalPath

	if err := wp.converter.Thumbnail(sourcePath, thumbPath); err != nil {
		return fmt.Errorf("thumbnail: %w", err)
	}

	media.ThumbPath = thumbPath
	return wp.store.UpdateDone(media)
}

func (wp *WorkerPool) handleProbe(job *domain.Job) error {
	media, err := wp.store.Get(job.MediaID)
	if err != nil {
		return fmt.Errorf("get media: %w", err)
	}

	sourcePath := media.ConvertedPath
	if sourcePath == "" {
		sourcePath = media.OriginalPath
	}

	probeResult, err := wp.converter.Probe(sourcePath)
	if err != nil {
		return fmt.Errorf("probe: %w", err)
	}

	width, height := probeResult.Dimensions()
	media.Width = width
	media.Height = height
	return wp.store.UpdateDone(media)
}

func (wp *WorkerPool) publishEvent(mediaID, eventType, status, message string) {
	if wp.eventBus != nil {
		wp.eventBus.Publish(mediaID, Event{
			Type:    eventType,
			Status:  status,
			Message: message,
		})
	}
}
