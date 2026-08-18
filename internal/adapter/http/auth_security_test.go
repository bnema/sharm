package http

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetClientID_UsesRemoteAddressWithoutTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "198.51.100.8:4567"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")

	assert.Equal(t, "198.51.100.8", getClientID(req, false, nil))
}

func TestSVGUploadsCannotInheritInlineSVGHandling(t *testing.T) {
	assert.Equal(t, domain.MediaTypeVideo, domain.DetectMediaType("payload.svg"))
	assert.Equal(t, "application/octet-stream", detectOriginalMIMEType(&domain.Media{
		Type:         domain.MediaTypeVideo,
		OriginalName: "payload.svg",
	}))
}

func TestGetClientID_UsesTrustedProxyHeadersWhenConfigured(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "127.0.0.1:4567"
	req.Header.Set("X-Real-IP", "198.51.100.8")
	req.Header.Set("X-Forwarded-For", "203.0.113.10")

	assert.Equal(t, "198.51.100.8", getClientID(req, true, loopbackProxyCIDRs(t)))
}

func TestGetClientID_RejectsForwardedHeadersFromUntrustedPeer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "198.51.100.8:4567"
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")

	assert.Equal(t, "198.51.100.8", getClientID(req, true, loopbackProxyCIDRs(t)))
}

func TestGetClientID_UsesConfiguredProxyNetwork(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "10.42.0.7:4567"
	req.Header.Set("X-Real-IP", "198.51.100.8")

	_, proxyNetwork, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)
	assert.Equal(t, "198.51.100.8", getClientID(req, true, []*net.IPNet{proxyNetwork}))
}

func TestSetupHandler_AllowsRemoteFirstRun(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/setup", http.NoBody)
	req.RemoteAddr = "198.51.100.8:4567"
	rec := httptest.NewRecorder()

	SetupHandler(configTestAuthService{}, "dev", false).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestUnauthenticatedHandlersRejectOversizedAndMultipartForms(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		hasUser bool
	}{
		{name: "login", path: "/login", hasUser: true},
		{name: "setup", path: "/setup", hasUser: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, form := range []struct {
				name        string
				contentType string
				body        string
			}{
				{name: "oversized urlencoded", contentType: "application/x-www-form-urlencoded", body: strings.Repeat("a", authFormBodyLimit+1)},
				{name: "multipart", contentType: "multipart/form-data; boundary=ignored", body: "ignored"},
			} {
				t.Run(form.name, func(t *testing.T) {
					server := newTestServer(configTestAuthService{hasUser: tt.hasUser}, configTestMediaService{})
					req := httptest.NewRequest(http.MethodPost, tt.path, strings.NewReader(form.body))
					req.Header.Set("Content-Type", form.contentType)
					token := server.csrf.GenerateToken()
					req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
					req.Header.Set("X-CSRF-Token", token)

					rec := httptest.NewRecorder()
					server.ServeHTTP(rec, req)

					assert.Equal(t, http.StatusBadRequest, rec.Code)
				})
			}
		})
	}
}

func loopbackProxyCIDRs(t *testing.T) []*net.IPNet {
	_, network, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	return []*net.IPNet{network}
}
