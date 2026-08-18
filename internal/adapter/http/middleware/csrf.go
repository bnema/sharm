package middleware

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"mime"
	"net/http"
	"strings"
)

const (
	csrfCookieName    = "csrf_token"
	csrfHeaderName    = "X-CSRF-Token"
	csrfFormField     = "csrf_token"
	csrfCookiePath    = "/"
	csrfMaxAge        = 86400 // 24 hours
	tokenSize         = 32    // 32 bytes random data
	csrfFormBodyLimit = 64 << 10
)

// CSRFErrorHandler is called when CSRF validation fails.
// It receives the response writer, request, and a fresh token
// that has already been set as a cookie on the response.
type CSRFErrorHandler func(w http.ResponseWriter, r *http.Request, newToken string)

// CSRFProtection provides CSRF token protection middleware.
type CSRFProtection struct {
	secretKey    []byte
	errorHandler CSRFErrorHandler
}

// NewCSRFProtection creates a new CSRF protection instance.
func NewCSRFProtection(secretKey string, errorHandler CSRFErrorHandler) *CSRFProtection {
	return &CSRFProtection{
		secretKey:    []byte(secretKey),
		errorHandler: errorHandler,
	}
}

// Middleware returns an HTTP middleware that enforces CSRF protection.
// Safe methods (GET, HEAD, OPTIONS) do not require token validation.
// Unsafe methods (POST, PUT, PATCH, DELETE) require a valid token.
func (c *CSRFProtection) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ensure a valid CSRF cookie exists. Replace it if missing
		// or if the signature doesn't match the current secret key
		// (e.g. after a server restart with a new SECRET_KEY).
		needNewToken := true
		if cookie, err := r.Cookie(csrfCookieName); err == nil {
			needNewToken = !c.ValidateToken(cookie.Value)
		}
		if needNewToken {
			token := c.GenerateToken()
			c.setCSRFCookie(w, r, token)
		}

		// Safe methods don't require token validation
		if isSafeMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}

		// Unsafe methods require token validation
		if !c.validateRequest(w, r) {
			// Flush the bad cookie and issue a fresh token so the
			// next request succeeds without a full page refresh.
			newToken := c.GenerateToken()
			c.setCSRFCookie(w, r, newToken)
			c.errorHandler(w, r, newToken)
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
func (c *CSRFProtection) validateRequest(w http.ResponseWriter, r *http.Request) bool {
	// Get token from cookie
	cookie, err := r.Cookie(csrfCookieName)
	if err != nil {
		return false
	}
	cookieToken := cookie.Value

	// Get token from request (header takes precedence)
	mediaType, _, mediaTypeErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaTypeErr == nil && strings.EqualFold(mediaType, "application/x-www-form-urlencoded") && r.Body != nil {
		// Cap URL-encoded bodies even when the token arrives in a header. The
		// downstream unauthenticated handlers may still call FormValue after the
		// header bypass, while multipart upload limits belong to those handlers.
		r.Body = http.MaxBytesReader(w, r.Body, csrfFormBodyLimit)
	}

	requestToken := r.Header.Get(csrfHeaderName)
	if requestToken == "" {
		// Never parse multipart bodies in global middleware. Upload handlers apply
		// their own size limits after authentication; parsing here could let an
		// unauthenticated request consume temporary disk first.
		if mediaTypeErr != nil || !strings.EqualFold(mediaType, "application/x-www-form-urlencoded") || r.Body == nil {
			return false
		}
		r.Body = http.MaxBytesReader(w, r.Body, csrfFormBodyLimit)
		if err := r.ParseForm(); err != nil {
			return false
		}
		requestToken = r.PostForm.Get(csrfFormField)
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
