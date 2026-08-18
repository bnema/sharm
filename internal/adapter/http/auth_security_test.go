package http

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bnema/sharm/internal/domain"
	"github.com/stretchr/testify/assert"
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

	assert.Equal(t, "198.51.100.8", getClientID(req, true, loopbackProxyCIDRs()))
}

func TestGetClientID_RejectsForwardedHeadersFromUntrustedPeer(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "198.51.100.8:4567"
	req.Header.Set("X-Real-IP", "127.0.0.1")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")

	assert.Equal(t, "198.51.100.8", getClientID(req, true, loopbackProxyCIDRs()))
}

func TestGetClientID_UsesConfiguredProxyNetwork(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/login", http.NoBody)
	req.RemoteAddr = "10.42.0.7:4567"
	req.Header.Set("X-Real-IP", "198.51.100.8")

	_, proxyNetwork, err := net.ParseCIDR("10.0.0.0/8")
	assert.NoError(t, err)
	assert.Equal(t, "198.51.100.8", getClientID(req, true, []*net.IPNet{proxyNetwork}))
}

func TestSetupHandler_AllowsRemoteFirstRun(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/setup", http.NoBody)
	req.RemoteAddr = "198.51.100.8:4567"
	rec := httptest.NewRecorder()

	SetupHandler(configTestAuthService{}, "dev", false).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func loopbackProxyCIDRs() []*net.IPNet {
	_, network, _ := net.ParseCIDR("127.0.0.0/8")
	return []*net.IPNet{network}
}
