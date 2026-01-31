package port

import "github.com/bnema/sharm/internal/domain"

type MediaStore interface {
	Save(m *domain.Media) error
	Get(id string) (*domain.Media, error)
	Delete(id string) error
	ListExpired() ([]*domain.Media, error)
	ListAll() ([]*domain.Media, error)
	UpdateStatus(id string, status domain.MediaStatus, errMsg string) error
	UpdateDone(m *domain.Media) error
	UpdateProbeJSON(id string, probeJSON string) error

	// Variant methods
	SaveVariant(v *domain.Variant) error
	GetVariant(id int64) (*domain.Variant, error)
	GetVariantByMediaAndCodec(mediaID string, codec domain.Codec) (*domain.Variant, error)
	ListVariantsByMedia(mediaID string) ([]domain.Variant, error)
	UpdateVariantStatus(id int64, status domain.VariantStatus, errMsg string) error
	UpdateVariantDone(v *domain.Variant) error
	DeleteVariantsByMedia(mediaID string) error
}
