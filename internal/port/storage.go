package port

import "github.com/bnema/sharm/internal/domain"

type MediaStore interface {
	Save(m *domain.Media) error
	Get(id string) (*domain.Media, error)
	Delete(id string) error
	ListExpired() ([]*domain.Media, error)
}
