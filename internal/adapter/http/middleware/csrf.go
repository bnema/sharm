package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strings"
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
			// Flush the bad cookie and issue a fresh token so the
			// next request succeeds without a full page refresh.
			newToken := c.GenerateToken()
			c.setCSRFCookie(w, r, newToken)

			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusForbidden)
			if strings.Contains(r.Header.Get("HX-Request"), "true") {
				// Return an error toast + a script that updates the
				// HTMX CSRF header with the fresh token from the cookie.
				_, _ = w.Write([]byte(
					`<div style="display:flex;align-items:center;gap:var(--s-sm);padding:var(--s-sm) var(--s-md);border-radius:var(--radius-md);font-size:var(--text-sm);border:1px solid;margin-bottom:var(--s-md);background:color-mix(in srgb,var(--error) 8%,var(--bg-surface));border-color:color-mix(in srgb,var(--error) 25%,transparent);color:var(--error);">` +
						`<svg width="14" height="14" viewBox="0 0 16 16" fill="currentColor"><path d="M8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0zM5.47 5.47a.75.75 0 0 0 0 1.06L6.94 8 5.47 9.47a.75.75 0 1 0 1.06 1.06L8 9.06l1.47 1.47a.75.75 0 1 0 1.06-1.06L9.06 8l1.47-1.47a.75.75 0 0 0-1.06-1.06L8 6.94 6.53 5.47a.75.75 0 0 0-1.06 0z"/></svg>` +
						`<span>Invalid CSRF token. Please try again.</span></div>` +
						`<script>` +
						`var c=document.cookie.split('; ').find(function(r){return r.startsWith('csrf_token=')});` +
						`if(c){document.body.setAttribute('hx-headers',JSON.stringify({'X-CSRF-Token':c.substring('csrf_token='.length)}));}` +
						`</script>`))
			} else {
				http.Error(w, "Forbidden", http.StatusForbidden)
			}
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

	// Tokens must match using constant-time comparison
	if !hmac.Equal([]byte(requestToken), []byte(cookieToken)) {
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
