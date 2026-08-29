package http

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type uploadHandlerTestService struct {
	chunkSize int64
}

func (s *uploadHandlerTestService) GetChunkSize() int64 { return s.chunkSize }
func (*uploadHandlerTestService) CreateSession(service.CreateUploadInput) (*domain.UploadSession, error) {
	return nil, domain.ErrInvalidUpload
}
func (*uploadHandlerTestService) GetSession(int64, string) (*domain.UploadSession, error) {
	return nil, domain.ErrNotFound
}
func (*uploadHandlerTestService) WriteChunk(_ int64, _, _ string, _ int, _ string, body io.Reader) (*domain.UploadChunk, error) {
	_, err := io.ReadAll(body)
	return nil, err
}
func (*uploadHandlerTestService) FinalizeAsset(int64, string, string) (*service.FinalizeUploadResult, error) {
	return nil, domain.ErrInvalidUpload
}
func (*uploadHandlerTestService) CancelSession(int64, string) error { return nil }

func TestUploadPageExposesConfiguredClientSizeLimit(t *testing.T) {
	handlers := NewHandlers(nil, nil, "", 768, "test", nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/upload", http.NoBody)

	handlers.UploadPage().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `data-max-upload-size-mb="768"`)
}

func TestUploadSessionChunkUsesConfiguredSizeAndReturnsPayloadTooLarge(t *testing.T) {
	handlers := NewHandlers(nil, nil, "", 10, "test", &uploadHandlerTestService{chunkSize: 4})
	request := httptest.NewRequest(http.MethodPut, "/upload/session/session-1/assets/asset-1/chunks/0", strings.NewReader("123456"))
	request.SetPathValue("sessionID", "session-1")
	request.SetPathValue("assetID", "asset-1")
	request.SetPathValue("index", "0")
	request = request.WithContext(context.WithValue(request.Context(), userKey, &domain.User{ID: 1}))
	recorder := httptest.NewRecorder()

	handlers.UploadSessionChunk().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusRequestEntityTooLarge, recorder.Code)
}

func TestUploadHandlersTreatTypedNilServiceAsUnavailable(t *testing.T) {
	var uploadSvc *service.UploadService
	handlers := NewHandlers(nil, nil, "", 10, "test", uploadSvc)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/upload/session", http.NoBody)

	handlers.CreateUploadSession().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusNotImplemented, recorder.Code)
}

func TestUploadDTOsDoNotExposeStoragePathsOrDeclaredHashes(t *testing.T) {
	session := &domain.UploadSession{Assets: []domain.UploadAsset{{
		ID:             "asset-1",
		Path:           "internal-storage-path",
		ExpectedSHA256: "declared-secret-hash",
		SHA256:         "stored-secret-hash",
	}}}
	payload, err := json.Marshal(uploadSessionResponse{Session: newUploadSessionDTO(session)})
	require.NoError(t, err)
	body := string(payload)

	assert.NotContains(t, body, "internal-storage-path")
	assert.NotContains(t, body, "declared-secret-hash")
	assert.NotContains(t, body, "stored-secret-hash")
}

func TestVariantDTODoesNotExposeStoragePath(t *testing.T) {
	payload, err := json.Marshal(newVariantDTO(&domain.Variant{Path: "internal-variant-path"}))
	require.NoError(t, err)

	assert.NotContains(t, string(payload), "internal-variant-path")
}
