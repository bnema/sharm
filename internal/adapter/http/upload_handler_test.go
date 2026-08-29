package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUploadPageExposesConfiguredClientSizeLimit(t *testing.T) {
	handlers := NewHandlers(nil, nil, "", 768, "test")
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/upload", http.NoBody)

	handlers.UploadPage().ServeHTTP(recorder, request)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Contains(t, recorder.Body.String(), `data-max-upload-size-mb="768"`)
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
