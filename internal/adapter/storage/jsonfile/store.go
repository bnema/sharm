package jsonfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

type Store struct {
	mu    sync.RWMutex
	path  string
	media map[string]*domain.Media
}

func NewStore(dataDir string) (*Store, error) {
	path := filepath.Join(dataDir, "media.json")

	store := &Store{
		path:  path,
		media: make(map[string]*domain.Media),
	}

	if err := store.load(); err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	return store, nil
}

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}

	if len(data) == 0 {
		return nil
	}

	var mediaList []*domain.Media
	if err := json.Unmarshal(data, &mediaList); err != nil {
		return err
	}

	for _, m := range mediaList {
		s.media[m.ID] = m
	}

	return nil
}

func (s *Store) save() error {
	tmpPath := s.path + ".tmp"

	mediaList := make([]*domain.Media, 0, len(s.media))
	for _, m := range s.media {
		mediaList = append(mediaList, m)
	}

	data, err := json.MarshalIndent(mediaList, "", "  ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path)
}

func (s *Store) Save(m *domain.Media) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.media[m.ID] = m
	return s.save()
}

func (s *Store) Get(id string) (*domain.Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	m, ok := s.media[id]
	if !ok {
		return nil, domain.ErrNotFound
	}

	return m, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.media, id)
	return s.save()
}

func (s *Store) ListExpired() ([]*domain.Media, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var expired []*domain.Media
	for _, m := range s.media {
		if m.IsExpired() {
			expired = append(expired, m)
		}
	}

	return expired, nil
}

var _ port.MediaStore = (*Store)(nil)
