package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
)

const (
	csrfCookieName = "csrf_token"
	csrfHeaderName = "X-CSRF-Token"
	csrfFormField  = "csrf_token"
	csrfCookiePath = "/"
	csrfMaxAge     = 86400 // 24 hours
	tokenSize      = 32    // 32 bytes random data
)

// CSRFProtection provides CSRF token protection middleware.
type CSRFProtection struct {
	secretKey []byte
}

// NewCSRFProtection creates a new CSRF protection instance.
func NewCSRFProtection(secretKey string) *CSRFProtection {
	return &CSRFProtection{
		secretKey: []byte(secretKey),
	}
}

// Middleware returns an HTTP middleware that enforces CSRF protection.
// Safe methods (GET, HEAD, OPTIONS) do not require token validation.
// Unsafe methods (POST, PUT, PATCH, DELETE) require a valid token.
func (c *CSRFProtection) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if we need to set a new token cookie
		if _, err := r.Cookie(csrfCookieName); err != nil {
			// No valid cookie, generate new token
			token := c.GenerateToken()
			c.setCSRFCookie(w, r, token)
		}

		// Safe methods don't require token validation
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Unsafe methods require token validation
		if !c.validateRequest(r) {
			http.Error(w, "Forbidden - Invalid CSRF token", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// GenerateToken creates a new CSRF token with HMAC signature.
// Token format: base64(32 random bytes + 32 bytes HMAC-SHA256 signature)
func (c *CSRFProtection) GenerateToken() string {
	randomBytes := make([]byte, tokenSize)
	if _, err := rand.Read(randomBytes); err != nil {
		// In case of crypto/rand failure, use less random but still functional
		// This should never happen in practice
		for i := range randomBytes {
			randomBytes[i] = byte(i)
		}
	}

	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write(randomBytes)
	signature := mac.Sum(nil)

	token := make([]byte, tokenSize+len(signature))
	copy(token[:tokenSize], randomBytes)
	copy(token[tokenSize:], signature)

	return base64.URLEncoding.EncodeToString(token)
}

// ValidateToken checks if a token has a valid HMAC signature.
func (c *CSRFProtection) ValidateToken(token string) bool {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil {
		return false
	}

	// Token must be exactly 64 bytes (32 random + 32 signature)
	if len(decoded) != 64 {
		return false
	}

	randomBytes := decoded[:tokenSize]
	providedSignature := decoded[tokenSize:]

	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write(randomBytes)
	expectedSignature := mac.Sum(nil)

	return hmac.Equal(providedSignature, expectedSignature)
}

// validateRequest checks if the request contains a valid CSRF token
// that matches the token in the cookie.
func (c *CSRFProtection) validateRequest(r *http.Request) bool {
	// Get token from cookie
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	cookieToken := cookie.Value

	// Get token from request (header takes precedence)
	requestToken := r.Header.Get(csrfHeaderName)
	if requestToken == "" {
		// Fall back to form field
		requestToken = r.FormValue(csrfFormField)
	}

	if requestToken == "" {
		return false
	}

	// Tokens must match
	if requestToken != cookieToken {
		return false
	}

	// Validate the token signature
	return c.ValidateToken(requestToken)
}

// setCSRFCookie sets the CSRF token cookie on the response.
func (c *CSRFProtection) setCSRFCookie(w http.ResponseWriter, r *http.Request, token string) {
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"

	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     csrfCookiePath,
		MaxAge:   csrfMaxAge,
		Secure:   secure,
		HttpOnly: false, // Must be readable by JavaScript for HTMX
		SameSite: http.SameSiteStrictMode,
	})
}

// isSafeMethod returns true for HTTP methods that don't require CSRF protection.
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
