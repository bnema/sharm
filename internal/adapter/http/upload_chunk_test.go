package http

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bnema/sharm/internal/adapter/http/middleware"
	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/adapter/storage/osfs"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/service"
)

const uploadChunkTestCSRFCookieName = "csrf_token"

type uploadChunkTestLogger struct{}

func (uploadChunkTestLogger) Infof(string, ...any)  {}
func (uploadChunkTestLogger) Errorf(string, ...any) {}
func (uploadChunkTestLogger) Debugf(string, ...any) {}
func (uploadChunkTestLogger) Warnf(string, ...any)  {}

type uploadChunkTestMediaService struct {
	uploadedName string
	uploadedBody []byte
	uploadErr    error
}

func (m *uploadChunkTestMediaService) Upload(name string, file *os.File, retention int, mediaType domain.MediaType, codecs []domain.Codec, fps int) (*domain.Media, error) {
	if m.uploadErr != nil {
		return nil, m.uploadErr
	}

	body, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	m.uploadedName = name
	m.uploadedBody = body
	return &domain.Media{ID: "media-1"}, nil
}

func (*uploadChunkTestMediaService) Get(string) (*domain.Media, error) {
	return nil, nil
}

func (*uploadChunkTestMediaService) ListAll() ([]*domain.Media, error) {
	return nil, nil
}

func (*uploadChunkTestMediaService) Delete(string) error {
	return nil
}

func (*uploadChunkTestMediaService) ProbeFile(string) (*domain.ProbeResult, error) {
	return nil, nil
}

func newUploadChunkTestServer(t *testing.T, mediaSvc *uploadChunkTestMediaService, tempDir string) *Server {
	t.Helper()

	chunkSvc := service.NewChunkService(tempDir, uploadChunkTestLogger{}, osfs.New())

	return NewServer(ServerConfig{
		AuthSvc:         configTestAuthService{hasUser: true},
		MediaSvc:        mediaSvc,
		ChunkSvc:        chunkSvc,
		EventBus:        service.NewEventBus(),
		Domain:          "example.com",
		MaxUploadSizeMB: 10,
		Version:         "dev",
		RateLimiter:     ratelimit.NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute),
		BackoffTracker:  ratelimit.NewLoginAttemptTracker(),
		Backoff:         ratelimit.NewBackoff(500*time.Millisecond, 10*time.Second, 2.0),
		CSRF:            middleware.NewCSRFProtection("test-secret", CSRFErrorHandler),
	})
}

func addUploadTestAuthAndCSRF(req *http.Request, server *Server) {
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "valid-token"})

	csrfToken := server.csrf.GenerateToken()
	req.AddCookie(&http.Cookie{Name: uploadChunkTestCSRFCookieName, Value: csrfToken})
	req.Header.Set("X-CSRF-Token", csrfToken)
}

func postChunk(t *testing.T, server *Server, uploadID string, chunkIndex int, body []byte) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	require.NoError(t, writer.WriteField("uploadId", uploadID))
	require.NoError(t, writer.WriteField("chunkIndex", strconv.Itoa(chunkIndex)))

	part, err := writer.CreateFormFile("chunk", "chunk.bin")
	require.NoError(t, err)

	_, err = part.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload/chunk", &requestBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addUploadTestAuthAndCSRF(req, server)

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

func postComplete(t *testing.T, server *Server, uploadID, filename string, totalChunks int) *httptest.ResponseRecorder {
	t.Helper()

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	require.NoError(t, writer.WriteField("uploadId", uploadID))
	require.NoError(t, writer.WriteField("filename", filename))
	require.NoError(t, writer.WriteField("totalChunks", strconv.Itoa(totalChunks)))
	require.NoError(t, writer.WriteField("retention", "7"))
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(http.MethodPost, "/upload/complete", &requestBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	addUploadTestAuthAndCSRF(req, server)

	rr := httptest.NewRecorder()
	server.ServeHTTP(rr, req)

	return rr
}

func TestChunkUpload_AllowsDuplicateChunkRetry(t *testing.T) {
	tempDir := t.TempDir()
	server := newUploadChunkTestServer(t, &uploadChunkTestMediaService{}, tempDir)

	first := postChunk(t, server, "upload-123", 0, []byte("chunk-a"))
	second := postChunk(t, server, "upload-123", 0, []byte("chunk-a"))

	assert.Equal(t, http.StatusOK, first.Code)
	assert.Equal(t, http.StatusOK, second.Code)
}

func TestCompleteUpload_ResumableFlowCleansUpChunks(t *testing.T) {
	tempDir := t.TempDir()
	mediaSvc := &uploadChunkTestMediaService{}
	server := newUploadChunkTestServer(t, mediaSvc, tempDir)

	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}

	firstChunk := postChunk(t, server, "upload-456", 0, pngBytes[:8])
	secondChunk := postChunk(t, server, "upload-456", 1, pngBytes[8:])
	require.Equal(t, http.StatusOK, firstChunk.Code)
	require.Equal(t, http.StatusOK, secondChunk.Code)

	rr := postComplete(t, server, "upload-456", "image.png", 2)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "/", rr.Header().Get("HX-Redirect"))
	assert.Equal(t, "image.png", mediaSvc.uploadedName)
	assert.Equal(t, pngBytes, mediaSvc.uploadedBody)
	assert.NoDirExists(t, filepath.Join(tempDir, "sharm-chunks", "upload-456"))
}

func TestCompleteUpload_FailedUploadStillCleansUpChunksAndAssembledTempFile(t *testing.T) {
	assembledTempDir := t.TempDir()
	t.Setenv("TMPDIR", assembledTempDir)

	tempDir := t.TempDir()
	mediaSvc := &uploadChunkTestMediaService{uploadErr: errors.New("boom")}
	server := newUploadChunkTestServer(t, mediaSvc, tempDir)

	pngBytes := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 'I', 'H', 'D', 'R'}

	require.Equal(t, http.StatusOK, postChunk(t, server, "upload-789", 0, pngBytes[:8]).Code)
	require.Equal(t, http.StatusOK, postChunk(t, server, "upload-789", 1, pngBytes[8:]).Code)

	rr := postComplete(t, server, "upload-789", "image.png", 2)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.NoDirExists(t, filepath.Join(tempDir, "sharm-chunks", "upload-789"))

	assembledFiles, err := filepath.Glob(filepath.Join(assembledTempDir, "upload-assembled-*.tmp"))
	require.NoError(t, err)
	assert.Empty(t, assembledFiles)
}
