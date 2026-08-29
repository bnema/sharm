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
	UpdateOriginalPath(id, path string) error
	UpdateProbeJSON(id string, probeJSON string) error
	SaveMediaAsset(asset *domain.MediaAsset) error
	GetMediaAsset(mediaID string, role domain.AssetRole) (*domain.MediaAsset, error)
	DeleteMediaAsset(mediaID string, role domain.AssetRole) error

	// Variant methods
	SaveVariant(v *domain.Variant) error
	PublishPrimaryVariant(media *domain.Media, variant *domain.Variant, probeJSON string) error
	GetVariant(id int64) (*domain.Variant, error)
	GetVariantByMediaAndCodec(mediaID string, codec domain.Codec) (*domain.Variant, error)
	ListVariantsByMedia(mediaID string) ([]domain.Variant, error)
	UpdateVariantStatus(id int64, status domain.VariantStatus, errMsg string) error
	UpdateVariantProgress(id int64, status domain.VariantStatus, progress int, errMsg string) error
	UpdateVariantDone(v *domain.Variant) error
	DeleteVariantsByMedia(mediaID string) error
}
