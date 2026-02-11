package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetAuthCookie_UsesLaxSameSite(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/login", http.NoBody)
	rr := httptest.NewRecorder()

	setAuthCookie(rr, req, "token-123", false)
	res := rr.Result()
	defer res.Body.Close()

	cookies := res.Cookies()
	require.NotEmpty(t, cookies)

	var authCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == CookieName {
			authCookie = c
			break
		}
	}
	require.NotNil(t, authCookie)

	assert.Equal(t, http.SameSiteLaxMode, authCookie.SameSite)
	assert.Equal(t, CookiePath, authCookie.Path)
	assert.Equal(t, CookieMaxAge, authCookie.MaxAge)
	assert.True(t, authCookie.HttpOnly)
}
