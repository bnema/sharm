package service

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/infrastructure/logger"
	"github.com/bnema/sharm/internal/port"
)

type MediaService struct {
	store     port.MediaStore
	converter port.MediaConverter
	uploadDir string
}

func NewMediaService(store port.MediaStore, converter port.MediaConverter, dataDir string) *MediaService {
	return &MediaService{
		store:     store,
		converter: converter,
		uploadDir: filepath.Join(dataDir, "uploads"),
	}
}

func (s *MediaService) Upload(filename string, file *os.File, retentionDays int) (*domain.Media, error) {
	if err := os.MkdirAll(s.uploadDir, 0755); err != nil {
		logger.Error.Printf("failed to create upload directory: %v", err)
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	uploadPath := filepath.Join(s.uploadDir, filename)
	if err := os.Rename(file.Name(), uploadPath); err != nil {
		logger.Error.Printf("failed to save upload %s: %v", filename, err)
		return nil, fmt.Errorf("failed to save upload: %w", err)
	}

	media := domain.NewMedia(domain.MediaTypeVideo, filename, uploadPath, retentionDays)

	if err := s.store.Save(media); err != nil {
		os.Remove(uploadPath)
		logger.Error.Printf("failed to save media metadata %s: %v", media.ID, err)
		return nil, fmt.Errorf("failed to save media metadata: %w", err)
	}

	logger.Info.Printf("media uploaded: id=%s, filename=%s, retention=%d days", media.ID, filename, retentionDays)
	go s.convert(media)

	return media, nil
}

func (s *MediaService) convert(media *domain.Media) {
	convertedDir := filepath.Join(filepath.Dir(s.uploadDir), "converted")
	if err := os.MkdirAll(convertedDir, 0755); err != nil {
		media.MarkAsFailed(fmt.Errorf("failed to create converted directory: %w", err))
		s.store.Save(media)
		return
	}

	convertedPath, codec, err := s.converter.Convert(media.OriginalPath, convertedDir, media.ID)
	if err != nil {
		media.MarkAsFailed(err)
		s.store.Save(media)
		return
	}

	width, height, err := s.converter.Probe(convertedPath)
	if err != nil {
		media.MarkAsFailed(fmt.Errorf("failed to probe video: %w", err))
		s.store.Save(media)
		return
	}

	thumbPath := filepath.Join(convertedDir, media.ID+"_thumb.jpg")
	if err := s.converter.Thumbnail(convertedPath, thumbPath); err != nil {
		media.MarkAsFailed(fmt.Errorf("failed to generate thumbnail: %w", err))
		s.store.Save(media)
		return
	}

	fileInfo, _ := os.Stat(convertedPath)
	media.MarkAsDone(convertedPath, domain.Codec(codec), width, height, thumbPath, fileInfo.Size())

	os.Remove(media.OriginalPath)

	s.store.Save(media)
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

func (s *MediaService) Cleanup() error {
	expired, err := s.store.ListExpired()
	if err != nil {
		return err
	}

	for _, media := range expired {
		os.Remove(media.OriginalPath)
		os.Remove(media.ConvertedPath)
		os.Remove(media.ThumbPath)
		s.store.Delete(media.ID)
	}

	return nil
}
