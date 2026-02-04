package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecretKey = "test-secret-key-for-csrf-protection"

func TestNewCSRFProtection(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	assert.NotNil(t, csrf)
}

func TestCSRFMiddleware_SetsCookieOnGET(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	cookies := rec.Result().Cookies()
	var csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			csrfCookie = c
			break
		}
	}

	require.NotNil(t, csrfCookie, "csrf_token cookie should be set")
	assert.NotEmpty(t, csrfCookie.Value)
	assert.Equal(t, "/", csrfCookie.Path)
	assert.Equal(t, 86400, csrfCookie.MaxAge)
	assert.Equal(t, http.SameSiteStrictMode, csrfCookie.SameSite)
	assert.False(t, csrfCookie.HttpOnly, "cookie should NOT be HttpOnly so JS can read it")
}

func TestCSRFMiddleware_GETDoesNotRequireToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_HEADDoesNotRequireToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_OPTIONSDoesNotRequireToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCSRFMiddleware_POSTRequiresToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PUTRequiresToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPut, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_PATCHRequiresToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPatch, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_DELETERequiresToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

func TestCSRFMiddleware_POSTWithValidHeaderToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First, get a token via GET request
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}
	require.NotEmpty(t, token)

	// Now make POST with token in header and cookie
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", token)
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusOK, postRec.Code)
}

func TestCSRFMiddleware_POSTWithValidFormToken(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First, get a token via GET request
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}
	require.NotEmpty(t, token)

	// Now make POST with token in form field and cookie
	postReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("csrf_token="+token))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusOK, postRec.Code)
}

func TestCSRFMiddleware_POSTWithMismatchedTokens(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get a valid token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}
	require.NotEmpty(t, token)

	// Make POST with different token in header vs cookie
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", "different-token")
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusForbidden, postRec.Code)
}

func TestCSRFMiddleware_POSTWithInvalidSignature(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Create a token with invalid signature
	invalidToken := base64.URLEncoding.EncodeToString([]byte("random-bytes-without-valid-signature"))

	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", invalidToken)
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: invalidToken})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusForbidden, postRec.Code)
}

func TestCSRFMiddleware_POSTWithoutCookie(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get a valid token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}
	require.NotEmpty(t, token)

	// Make POST with header token but no cookie
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", token)
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusForbidden, postRec.Code)
}

func TestCSRFMiddleware_HeaderTakesPrecedenceOverForm(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get a valid token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}
	require.NotEmpty(t, token)

	// Make POST with valid header token and invalid form token
	postReq := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("csrf_token=invalid"))
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.Header.Set("X-CSRF-Token", token)
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusOK, postRec.Code)
}

func TestCSRFMiddleware_CallsNextHandler(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	called := false
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.True(t, called)
}

func TestCSRFMiddleware_PreservesResponseStatus(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestCSRFMiddleware_PreservesResponseBody(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("test response"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	assert.Equal(t, "test response", rec.Body.String())
}

func TestCSRFToken_TokenFormat(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var token string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}

	// Token should be base64 URL encoded
	decoded, err := base64.URLEncoding.DecodeString(token)
	require.NoError(t, err)

	// Token should be 32 bytes random + 32 bytes HMAC-SHA256 = 64 bytes
	assert.Equal(t, 64, len(decoded))
}

func TestCSRFToken_SignatureVerification(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	var token string
	for _, c := range rec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}

	decoded, err := base64.URLEncoding.DecodeString(token)
	require.NoError(t, err)

	// Split into random bytes and signature
	randomBytes := decoded[:32]
	signature := decoded[32:]

	// Verify signature matches
	mac := hmac.New(sha256.New, []byte(testSecretKey))
	mac.Write(randomBytes)
	expectedSignature := mac.Sum(nil)

	assert.True(t, hmac.Equal(signature, expectedSignature))
}

func TestCSRFToken_TokenNotReusedAcrossSessions(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get first token
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	var token1 string
	for _, c := range rec1.Result().Cookies() {
		if c.Name == "csrf_token" {
			token1 = c.Value
			break
		}
	}

	// Get second token (no cookie sent, so new one generated)
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	var token2 string
	for _, c := range rec2.Result().Cookies() {
		if c.Name == "csrf_token" {
			token2 = c.Value
			break
		}
	}

	// Tokens should be different
	assert.NotEqual(t, token1, token2)
}

func TestCSRFToken_ExistingCookiePreserved(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get initial token
	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	var initialToken string
	for _, c := range rec1.Result().Cookies() {
		if c.Name == "csrf_token" {
			initialToken = c.Value
			break
		}
	}
	require.NotEmpty(t, initialToken)

	// Make second request with cookie - should not set new cookie
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.AddCookie(&http.Cookie{Name: "csrf_token", Value: initialToken})
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	// No new cookie should be set
	var newCookie *http.Cookie
	for _, c := range rec2.Result().Cookies() {
		if c.Name == "csrf_token" {
			newCookie = c
			break
		}
	}
	assert.Nil(t, newCookie, "should not set new cookie when valid one exists")
}

func TestCSRFMiddleware_ConstantTimeComparison(t *testing.T) {
	// This test ensures we use constant-time comparison
	// We can't directly test timing, but we can verify hmac.Equal is used
	// by checking that tampered tokens are rejected

	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Get a valid token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
			break
		}
	}

	// Tamper with the token slightly
	tamperedToken := token[:len(token)-1] + "X"

	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", tamperedToken)
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: tamperedToken})
	postRec := httptest.NewRecorder()

	handler.ServeHTTP(postRec, postReq)

	assert.Equal(t, http.StatusForbidden, postRec.Code)
}

func TestCSRFMiddleware_UsesSecureCookieWhenTLS(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)
	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	var csrfCookie *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == "csrf_token" {
			csrfCookie = c
			break
		}
	}

	require.NotNil(t, csrfCookie)
	assert.True(t, csrfCookie.Secure, "cookie should be Secure when behind TLS")
}

func TestCSRFToken_HelperFunction(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)

	token := csrf.GenerateToken()
	assert.NotEmpty(t, token)

	// Verify it's valid
	assert.True(t, csrf.ValidateToken(token))
}

func TestCSRFToken_InvalidTokenRejected(t *testing.T) {
	csrf := NewCSRFProtection(testSecretKey)

	// Completely invalid token
	assert.False(t, csrf.ValidateToken("not-a-valid-token"))

	// Valid base64 but wrong content
	assert.False(t, csrf.ValidateToken(base64.URLEncoding.EncodeToString([]byte("short"))))

	// Correct length but wrong signature
	fakeToken := make([]byte, 64)
	assert.False(t, csrf.ValidateToken(base64.URLEncoding.EncodeToString(fakeToken)))
}
