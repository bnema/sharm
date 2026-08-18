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

func TestSetupHandler_OnlyAllowsLoopbackClients(t *testing.T) {
	tests := []struct {
		name        string
		remoteAddr  string
		forwardedIP string
		behindProxy bool
		wantStatus  int
	}{
		{
			name:       "remote client",
			remoteAddr: "198.51.100.8:4567",
			wantStatus: http.StatusNotFound,
		},
		{
			name:       "local client",
			remoteAddr: "127.0.0.1:4567",
			wantStatus: http.StatusOK,
		},
		{
			name:        "remote client through proxy",
			remoteAddr:  "127.0.0.1:4567",
			forwardedIP: "198.51.100.8",
			behindProxy: true,
			wantStatus:  http.StatusNotFound,
		},
		{
			name:        "local client through proxy",
			remoteAddr:  "127.0.0.1:4567",
			forwardedIP: "127.0.0.1",
			behindProxy: true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "local tunnel through proxy without forwarded header",
			remoteAddr:  "127.0.0.1:4567",
			behindProxy: true,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "untrusted peer forging loopback header",
			remoteAddr:  "198.51.100.8:4567",
			forwardedIP: "127.0.0.1",
			behindProxy: true,
			wantStatus:  http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/setup", http.NoBody)
			req.RemoteAddr = tt.remoteAddr
			if tt.forwardedIP != "" {
				req.Header.Set("X-Real-IP", tt.forwardedIP)
			}

			rec := httptest.NewRecorder()
			SetupHandler(configTestAuthService{}, "dev", tt.behindProxy, loopbackProxyCIDRs()).ServeHTTP(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func loopbackProxyCIDRs() []*net.IPNet {
	_, network, _ := net.ParseCIDR("127.0.0.0/8")
	return []*net.IPNet{network}
}
