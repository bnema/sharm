package http

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/bnema/sharm/internal/adapter/http/middleware"
	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/service"
	"github.com/stretchr/testify/assert"
)

type configTestMediaService struct{}

func (configTestMediaService) Upload(string, *os.File, int, domain.MediaType, []domain.Codec, int) (*domain.Media, error) {
	return nil, nil
}

func (configTestMediaService) Get(string) (*domain.Media, error) {
	return nil, nil
}

func (configTestMediaService) ListAll() ([]*domain.Media, error) {
	return nil, nil
}

func (configTestMediaService) Delete(string) error {
	return nil
}

func (configTestMediaService) ProbeFile(string) (*domain.ProbeResult, error) {
	return nil, nil
}

type configTestAuthService struct {
	hasUser          bool
	validateTokenErr error
}

func (a configTestAuthService) HasUser() (bool, error) {
	return a.hasUser, nil
}

func (configTestAuthService) ValidatePassword(string, string) error {
	return nil
}

func (configTestAuthService) GenerateToken(string) (string, error) {
	return "", nil
}

func (a configTestAuthService) ValidateToken(string) (*domain.User, error) {
	if a.validateTokenErr != nil {
		return nil, a.validateTokenErr
	}
	return &domain.User{Username: "tester"}, nil
}

func (configTestAuthService) CreateUser(string, string) error {
	return nil
}

func (configTestAuthService) ChangePassword(string, string, string) error {
	return nil
}

func newTestServer(authSvc AuthService, mediaSvc MediaService) *Server {
	rl := ratelimit.NewLoginRateLimiter(5, 15*time.Minute, 30*time.Minute)
	bt := ratelimit.NewLoginAttemptTracker()
	bo := ratelimit.NewBackoff(500*time.Millisecond, 10*time.Second, 2.0)
	cs := middleware.NewCSRFProtection("test-secret")
	return NewServer(authSvc, mediaSvc, service.NewEventBus(), "example.com", 10, "dev", false, rl, bt, bo, cs)
}

func TestConfigPage_ReturnsRenderedConfigPage(t *testing.T) {
	h := NewHandlers(configTestMediaService{}, "example.com", 10, "dev")

	req := httptest.NewRequest(http.MethodGet, "/config", http.NoBody)
	rr := httptest.NewRecorder()

	h.ConfigPage()(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rr.Body.String(), "id=\"change-password-form\"")
	assert.Contains(t, rr.Body.String(), "id=\"pwa-install-btn\"")
}

func TestConfigRoute_RequiresAuthentication(t *testing.T) {
	authSvc := configTestAuthService{hasUser: true}
	s := newTestServer(authSvc, configTestMediaService{})

	req := httptest.NewRequest(http.MethodGet, "/config", http.NoBody)
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/login", rr.Header().Get("Location"))
}

func TestConfigRoute_AllowsAuthenticatedUser(t *testing.T) {
	authSvc := configTestAuthService{hasUser: true}
	s := newTestServer(authSvc, configTestMediaService{})

	req := httptest.NewRequest(http.MethodGet, "/config", http.NoBody)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "valid-token"})
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "hx-post=\"/change-password\"")
	assert.Contains(t, rr.Body.String(), "id=\"pwa-delete-btn\"")
}

func TestConfigRoute_RedirectsOnInvalidToken(t *testing.T) {
	authSvc := configTestAuthService{hasUser: true, validateTokenErr: errors.New("invalid token")}
	s := newTestServer(authSvc, configTestMediaService{})

	req := httptest.NewRequest(http.MethodGet, "/config", http.NoBody)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "bad-token"})
	rr := httptest.NewRecorder()

	s.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusSeeOther, rr.Code)
	assert.Equal(t, "/login", rr.Header().Get("Location"))
}
