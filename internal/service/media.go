package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"syscall"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

type MediaService struct {
	store     port.MediaStore
	converter port.MediaConverter
	jobQueue  port.JobQueue
	uploadDir string
	log       port.Logger
	fs        port.FileSystem
}

func NewMediaService(store port.MediaStore, converter port.MediaConverter, jobQueue port.JobQueue, dataDir string, log port.Logger, fs port.FileSystem) *MediaService {
	return &MediaService{
		store:     store,
		converter: converter,
		jobQueue:  jobQueue,
		uploadDir: filepath.Join(dataDir, "uploads"),
		log:       log,
		fs:        fs,
	}
}

func (s *MediaService) Upload(filename string, file *os.File, retentionDays int, mediaType domain.MediaType, codecs []domain.Codec, fps int) (*domain.Media, error) {
	if err := s.fs.MkdirAll(s.uploadDir, 0750); err != nil {
		s.log.Errorf("failed to create upload directory: %v", err)
		return nil, fmt.Errorf("failed to create upload directory: %w", categorizeIOError(err))
	}

	uploadPath := filepath.Join(s.uploadDir, filepath.Base(filename))

	err := s.fs.Rename(file.Name(), uploadPath)
	if err != nil {
		if isCrossDeviceError(err) {
			if copyErr := s.copyFile(file, uploadPath); copyErr != nil {
				s.log.Errorf("failed to copy upload %s: %v", filename, copyErr)
				return nil, fmt.Errorf("failed to copy upload: %w", categorizeIOError(copyErr))
			}
			_ = s.fs.Remove(file.Name())
		} else {
			s.log.Errorf("failed to save upload %s: %v", filename, err)
			return nil, fmt.Errorf("failed to save upload: %w", categorizeIOError(err))
		}
	}

	media := domain.NewMedia(mediaType, filename, uploadPath, retentionDays)

	finalUploadPath := filepath.Join(s.uploadDir, fmt.Sprintf("%s_%s", media.ID, filepath.Base(filename)))
	if err := s.fs.Rename(uploadPath, finalUploadPath); err != nil {
		s.log.Errorf("failed to rename upload with ID prefix: %v", err)
		_ = s.fs.Remove(uploadPath)
		return nil, fmt.Errorf("failed to finalize upload: %w", categorizeIOError(err))
	}
	media.OriginalPath = finalUploadPath

	probeResult, _ := s.converter.Probe(finalUploadPath)
	if probeResult != nil {
		rawJSON := probeResult.RawJSON
		if len(rawJSON) > 1*1024*1024 {
			rawJSON = rawJSON[:1*1024*1024]
		}
		media.ProbeJSON = rawJSON
		width, height := probeResult.Dimensions()
		media.Width = width
		media.Height = height
	}

	if err := s.store.Save(media); err != nil {
		_ = s.fs.Remove(uploadPath)
		s.log.Errorf("failed to save media metadata %s: %v", media.ID, err)
		return nil, fmt.Errorf("failed to save media metadata: %w", err)
	}

	s.log.Infof("media uploaded: id=%s, type=%s, filename=%s, retention=%d days, codecs=%v", media.ID, mediaType, filename, retentionDays, codecs)

	if mediaType == domain.MediaTypeImage {
		fileInfo, _ := s.fs.Stat(finalUploadPath)
		var fileSize int64
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}
		media.MarkAsDone(finalUploadPath, "", 0, 0, "", fileSize)
		if err := s.store.UpdateDone(media); err != nil {
			s.log.Errorf("failed to update image as done: %v", err)
		}
		return media, nil
	}

	// Ensure H264 is always included for video uploads (Discord/web compat)
	if mediaType == domain.MediaTypeVideo && !slices.Contains(codecs, domain.CodecH264) {
		codecs = append(codecs, domain.CodecH264)
	}

	if len(codecs) == 0 {
		fileInfo, _ := s.fs.Stat(finalUploadPath)
		var fileSize int64
		if fileInfo != nil {
			fileSize = fileInfo.Size()
		}
		media.MarkAsDone(finalUploadPath, "", 0, 0, "", fileSize)
		if err := s.store.UpdateDone(media); err != nil {
			s.log.Errorf("failed to update media as done: %v", err)
		}

		if mediaType == domain.MediaTypeVideo && s.jobQueue != nil {
			if _, err := s.jobQueue.Enqueue(media.ID, domain.JobTypeThumbnail, "", 0); err != nil {
				s.log.Errorf("failed to enqueue thumbnail job for %s: %v", media.ID, err)
			}
		}

		return media, nil
	}

	if s.jobQueue != nil {
		for _, codec := range codecs {
			v := &domain.Variant{
				MediaID: media.ID,
				Codec:   codec,
				Status:  domain.VariantStatusPending,
			}
			if err := s.store.SaveVariant(v); err != nil {
				s.log.Errorf("failed to save variant for %s codec %s: %v", media.ID, codec, err)
				continue
			}
			if _, err := s.jobQueue.Enqueue(media.ID, domain.JobTypeConvert, codec, fps); err != nil {
				s.log.Errorf("failed to enqueue convert job for %s codec %s: %v", media.ID, codec, err)
			}
		}
	}

	return media, nil
}

func (s *MediaService) Get(id string) (*domain.Media, error) {
	media, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}

	if media.IsExpired() {
		return nil, domain.ErrExpired
	}

	return media, nil
}

func (s *MediaService) ListAll() ([]*domain.Media, error) {
	return s.store.ListAll()
}

func (s *MediaService) Delete(id string) error {
	media, err := s.store.Get(id)
	if err != nil {
		return err
	}

	// Remove variant files
	for _, v := range media.Variants {
		if v.Path != "" {
			_ = s.fs.Remove(v.Path)
		}
	}

	// Remove files from disk
	if media.OriginalPath != "" {
		_ = s.fs.Remove(media.OriginalPath)
	}
	if media.ConvertedPath != "" {
		_ = s.fs.Remove(media.ConvertedPath)
	}
	if media.ThumbPath != "" {
		_ = s.fs.Remove(media.ThumbPath)
	}

	return s.store.Delete(id)
}

func (s *MediaService) Cleanup() error {
	expired, err := s.store.ListExpired()
	if err != nil {
		return err
	}

	for _, media := range expired {
		for _, v := range media.Variants {
			if v.Path != "" {
				_ = s.fs.Remove(v.Path)
			}
		}
		_ = s.fs.Remove(media.OriginalPath)
		_ = s.fs.Remove(media.ConvertedPath)
		_ = s.fs.Remove(media.ThumbPath)
		_ = s.store.Delete(media.ID)
	}

	return nil
}

func (s *MediaService) ProbeFile(filePath string) (*domain.ProbeResult, error) {
	return s.converter.Probe(filePath)
}

func categorizeIOError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, syscall.ENOSPC) {
		return fmt.Errorf("%w: %w", domain.ErrDiskFull, err)
	}
	if errors.Is(err, os.ErrPermission) {
		return fmt.Errorf("%w: %w", domain.ErrPermission, err)
	}
	return err
}

func isCrossDeviceError(err error) bool {
	return errors.Is(err, syscall.EXDEV)
}

func (s *MediaService) copyFile(src *os.File, dstPath string) error {
	srcFile, err := s.fs.Open(src.Name())
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close() //nolint:errcheck

	dstFile, err := s.fs.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close() //nolint:errcheck

	if _, copyErr := io.Copy(dstFile, srcFile); copyErr != nil {
		return fmt.Errorf("failed to copy file contents: %w", copyErr)
	}

	srcInfo, err := src.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source file: %w", err)
	}

	if err := s.fs.Chmod(dstPath, srcInfo.Mode()); err != nil {
		return fmt.Errorf("failed to set file permissions: %w", err)
	}

	return nil
}
