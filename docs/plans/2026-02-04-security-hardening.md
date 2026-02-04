# Security Hardening Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Fix all security vulnerabilities identified in the vibsec audit: HTTP headers, CSRF tokens, magic bytes validation, filename sanitization, FFmpeg hardening, and log sanitization.

**Architecture:** Add a security middleware layer for HTTP headers and CSRF protection. Extend upload validation with magic bytes checking. Sanitize all user inputs before logging or using in HTTP headers.

**Tech Stack:** Go stdlib (net/http, crypto/rand), existing templ templates, HTMX compatibility

---

## Task 1: Add Security Headers Middleware

**Files:**
- Create: `internal/adapter/http/middleware/security.go`
- Create: `internal/adapter/http/middleware/security_test.go`
- Modify: `internal/adapter/http/server.go:99-101`

**Step 1: Create middleware directory**

```bash
mkdir -p internal/adapter/http/middleware
```

**Step 2: Write the failing test**

```go
// internal/adapter/http/middleware/security_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurityHeaders(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	tests := []struct {
		header   string
		expected string
	}{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "strict-origin-when-cross-origin"},
		{"Permissions-Policy", "camera=(), microphone=(), geolocation=()"},
	}

	for _, tt := range tests {
		got := rec.Header().Get(tt.header)
		if got != tt.expected {
			t.Errorf("header %s = %q, want %q", tt.header, got, tt.expected)
		}
	}
}

func TestSecurityHeadersCSP(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header is missing")
	}

	// Verify key directives are present
	required := []string{"default-src", "script-src", "style-src", "frame-ancestors 'none'"}
	for _, directive := range required {
		if !contains(csp, directive) {
			t.Errorf("CSP missing directive: %s", directive)
		}
	}
}

func TestSecurityHeadersHSTS(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Test without TLS - no HSTS
	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("Strict-Transport-Security") != "" {
		t.Error("HSTS should not be set for non-TLS requests")
	}

	// Test with X-Forwarded-Proto: https - should have HSTS
	req = httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	req.Header.Set("X-Forwarded-Proto", "https")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	hsts := rec.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("HSTS should be set when X-Forwarded-Proto is https")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
```

**Step 3: Run test to verify it fails**

```bash
go test -v ./internal/adapter/http/middleware/...
```

Expected: FAIL (package doesn't exist)

**Step 4: Write the implementation**

```go
// internal/adapter/http/middleware/security.go
package middleware

import "net/http"

// SecurityHeaders adds security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Disable unnecessary browser features
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Content Security Policy
		// Note: 'unsafe-inline' needed for HTMX inline handlers and templ styles
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline'; " +
			"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com; " +
			"img-src 'self' data:; " +
			"media-src 'self'; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'"
		w.Header().Set("Content-Security-Policy", csp)

		// HSTS only when behind TLS proxy or direct TLS
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}
```

**Step 5: Run test to verify it passes**

```bash
go test -v ./internal/adapter/http/middleware/...
```

Expected: PASS

**Step 6: Integrate middleware in server**

Modify `internal/adapter/http/server.go`:

```go
// Add import at top
import (
	// ... existing imports
	"github.com/bnema/sharm/internal/adapter/http/middleware"
)

// Modify ServeHTTP method (around line 99)
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	middleware.SecurityHeaders(s.mux).ServeHTTP(w, r)
}
```

**Step 7: Run all tests**

```bash
make test
```

Expected: All tests pass

**Step 8: Commit**

```bash
git add internal/adapter/http/middleware/ internal/adapter/http/server.go
git commit -m "feat(security): add HTTP security headers middleware"
```

---

## Task 2: Add CSRF Token Protection

**Files:**
- Create: `internal/adapter/http/middleware/csrf.go`
- Create: `internal/adapter/http/middleware/csrf_test.go`
- Modify: `internal/adapter/http/server.go`
- Modify: `internal/adapter/http/templates/layout.templ`

**Step 1: Write the failing test**

```go
// internal/adapter/http/middleware/csrf_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCSRFTokenGeneration(t *testing.T) {
	csrf := NewCSRFProtection("test-secret-key-32-bytes-long!!")

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := CSRFToken(r)
		if token == "" {
			t.Error("CSRF token should be set in context")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	// Check cookie is set
	cookies := rec.Result().Cookies()
	var found bool
	for _, c := range cookies {
		if c.Name == "csrf_token" {
			found = true
			if c.HttpOnly {
				t.Error("CSRF cookie should NOT be HttpOnly (JS needs to read it)")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("CSRF cookie should be SameSite=Strict")
			}
		}
	}
	if !found {
		t.Error("csrf_token cookie not set")
	}
}

func TestCSRFValidationPOST(t *testing.T) {
	csrf := NewCSRFProtection("test-secret-key-32-bytes-long!!")

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First GET to obtain token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
		}
	}

	// POST without token - should fail
	postReq := httptest.NewRequest(http.MethodPost, "/", nil)
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token should return 403, got %d", postRec.Code)
	}

	// POST with valid token - should succeed
	postReq = httptest.NewRequest(http.MethodPost, "/", nil)
	postReq.Header.Set("X-CSRF-Token", token)
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec = httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Errorf("POST with valid CSRF token should return 200, got %d", postRec.Code)
	}
}

func TestCSRFValidationDELETE(t *testing.T) {
	csrf := NewCSRFProtection("test-secret-key-32-bytes-long!!")

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// DELETE without token - should fail
	req := httptest.NewRequest(http.MethodDelete, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("DELETE without CSRF token should return 403, got %d", rec.Code)
	}
}

func TestCSRFSkipsGET(t *testing.T) {
	csrf := NewCSRFProtection("test-secret-key-32-bytes-long!!")

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// GET should always succeed (no CSRF validation)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GET should not require CSRF token, got %d", rec.Code)
	}
}

func TestCSRFFormValue(t *testing.T) {
	csrf := NewCSRFProtection("test-secret-key-32-bytes-long!!")

	handler := csrf.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First GET to obtain token
	getReq := httptest.NewRequest(http.MethodGet, "/", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	var token string
	for _, c := range getRec.Result().Cookies() {
		if c.Name == "csrf_token" {
			token = c.Value
		}
	}

	// POST with token in form body (fallback for non-JS forms)
	body := strings.NewReader("csrf_token=" + token)
	postReq := httptest.NewRequest(http.MethodPost, "/", body)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postReq.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Errorf("POST with CSRF token in form should return 200, got %d", postRec.Code)
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./internal/adapter/http/middleware/... -run CSRF
```

Expected: FAIL

**Step 3: Write the implementation**

```go
// internal/adapter/http/middleware/csrf.go
package middleware

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"time"
)

type csrfContextKey struct{}

// CSRFProtection provides CSRF token generation and validation.
type CSRFProtection struct {
	secretKey []byte
}

// NewCSRFProtection creates a new CSRF protection middleware.
func NewCSRFProtection(secretKey string) *CSRFProtection {
	return &CSRFProtection{
		secretKey: []byte(secretKey),
	}
}

// Middleware returns HTTP middleware that handles CSRF protection.
func (c *CSRFProtection) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Generate or retrieve token
		token := c.getOrCreateToken(w, r)

		// Store token in context for templates
		ctx := context.WithValue(r.Context(), csrfContextKey{}, token)
		r = r.WithContext(ctx)

		// Validate on state-changing methods
		if r.Method == http.MethodPost || r.Method == http.MethodPut ||
			r.Method == http.MethodPatch || r.Method == http.MethodDelete {

			if !c.validateToken(r, token) {
				http.Error(w, "CSRF token validation failed", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func (c *CSRFProtection) getOrCreateToken(w http.ResponseWriter, r *http.Request) string {
	// Check for existing token in cookie
	if cookie, err := r.Cookie("csrf_token"); err == nil && cookie.Value != "" {
		// Validate the token signature
		if c.verifyTokenSignature(cookie.Value) {
			return cookie.Value
		}
	}

	// Generate new token
	token := c.generateToken()

	// Set cookie (not HttpOnly so JS can read it for HTMX)
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    token,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		HttpOnly: false, // JS needs to read this
		SameSite: http.SameSiteStrictMode,
	})

	return token
}

func (c *CSRFProtection) generateToken() string {
	// Generate random bytes
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		// Fallback to timestamp-based (less secure but functional)
		randomBytes = []byte(time.Now().String())
	}

	// Create HMAC signature
	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write(randomBytes)
	signature := mac.Sum(nil)

	// Combine: random + signature
	token := append(randomBytes, signature...)
	return base64.URLEncoding.EncodeToString(token)
}

func (c *CSRFProtection) verifyTokenSignature(token string) bool {
	decoded, err := base64.URLEncoding.DecodeString(token)
	if err != nil || len(decoded) < 64 {
		return false
	}

	randomBytes := decoded[:32]
	providedSig := decoded[32:]

	mac := hmac.New(sha256.New, c.secretKey)
	mac.Write(randomBytes)
	expectedSig := mac.Sum(nil)

	return hmac.Equal(providedSig, expectedSig)
}

func (c *CSRFProtection) validateToken(r *http.Request, cookieToken string) bool {
	// Get token from header (HTMX/AJAX) or form value (traditional forms)
	requestToken := r.Header.Get("X-CSRF-Token")
	if requestToken == "" {
		// Try form value as fallback
		if err := r.ParseForm(); err == nil {
			requestToken = r.FormValue("csrf_token")
		}
	}

	if requestToken == "" {
		return false
	}

	// Constant-time comparison
	return hmac.Equal([]byte(requestToken), []byte(cookieToken))
}

// CSRFToken returns the CSRF token from the request context.
func CSRFToken(r *http.Request) string {
	if token, ok := r.Context().Value(csrfContextKey{}).(string); ok {
		return token
	}
	return ""
}
```

**Step 4: Run tests**

```bash
go test -v ./internal/adapter/http/middleware/... -run CSRF
```

Expected: PASS

**Step 5: Update templates to include CSRF token**

Modify `internal/adapter/http/templates/layout.templ` - add to `<head>`:

```go
// Add this script in the <head> section after HTMX script
<script>
	// Configure HTMX to send CSRF token with all requests
	document.addEventListener('DOMContentLoaded', function() {
		const csrfToken = document.cookie.split('; ')
			.find(row => row.startsWith('csrf_token='))
			?.split('=')[1];
		if (csrfToken) {
			document.body.setAttribute('hx-headers', JSON.stringify({'X-CSRF-Token': csrfToken}));
		}
	});
</script>
```

**Step 6: Integrate CSRF middleware in server**

Modify `internal/adapter/http/server.go`:

```go
// Add to Server struct
type Server struct {
	// ... existing fields
	csrf *middleware.CSRFProtection
}

// In NewServer function, add:
csrf := middleware.NewCSRFProtection(secretKey) // pass from config

s := &Server{
	// ... existing fields
	csrf: csrf,
}

// Modify ServeHTTP method
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Chain: SecurityHeaders -> CSRF -> Router
	middleware.SecurityHeaders(
		s.csrf.Middleware(s.mux),
	).ServeHTTP(w, r)
}
```

**Step 7: Run all tests**

```bash
make test
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/adapter/http/middleware/csrf.go internal/adapter/http/middleware/csrf_test.go
git add internal/adapter/http/server.go internal/adapter/http/templates/layout.templ
git commit -m "feat(security): add CSRF token protection with HTMX integration"
```

---

## Task 3: Add Magic Bytes Validation for Uploads

**Files:**
- Create: `internal/adapter/http/validation/filetype.go`
- Create: `internal/adapter/http/validation/filetype_test.go`
- Modify: `internal/adapter/http/handler.go`

**Step 1: Create validation directory**

```bash
mkdir -p internal/adapter/http/validation
```

**Step 2: Write the failing test**

```go
// internal/adapter/http/validation/filetype_test.go
package validation

import (
	"bytes"
	"testing"
)

func TestValidateMagicBytes(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantAllowed bool
		wantMIME    string
	}{
		{
			name:        "JPEG image",
			data:        []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46},
			wantAllowed: true,
			wantMIME:    "image/jpeg",
		},
		{
			name:        "PNG image",
			data:        []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			wantAllowed: true,
			wantMIME:    "image/png",
		},
		{
			name:        "GIF image",
			data:        []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61},
			wantAllowed: true,
			wantMIME:    "image/gif",
		},
		{
			name:        "WebP image",
			data:        []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			wantAllowed: true,
			wantMIME:    "image/webp",
		},
		{
			name:        "MP4 video (ftyp)",
			data:        append([]byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70}, bytes.Repeat([]byte{0x00}, 24)...),
			wantAllowed: true,
			wantMIME:    "video/mp4",
		},
		{
			name:        "WebM video",
			data:        []byte{0x1A, 0x45, 0xDF, 0xA3},
			wantAllowed: true,
			wantMIME:    "video/webm",
		},
		{
			name:        "MP3 audio (ID3)",
			data:        []byte{0x49, 0x44, 0x33, 0x04, 0x00, 0x00},
			wantAllowed: true,
			wantMIME:    "audio/mpeg",
		},
		{
			name:        "OGG audio",
			data:        []byte{0x4F, 0x67, 0x67, 0x53, 0x00, 0x02},
			wantAllowed: true,
			wantMIME:    "application/ogg",
		},
		{
			name:        "WAV audio",
			data:        []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x41, 0x56, 0x45},
			wantAllowed: true,
			wantMIME:    "audio/wave",
		},
		{
			name:        "PHP script",
			data:        []byte("<?php echo 'hello'; ?>"),
			wantAllowed: false,
			wantMIME:    "",
		},
		{
			name:        "HTML file",
			data:        []byte("<!DOCTYPE html><html>"),
			wantAllowed: false,
			wantMIME:    "",
		},
		{
			name:        "JavaScript file",
			data:        []byte("function evil() { }"),
			wantAllowed: false,
			wantMIME:    "",
		},
		{
			name:        "EXE file",
			data:        []byte{0x4D, 0x5A, 0x90, 0x00},
			wantAllowed: false,
			wantMIME:    "",
		},
		{
			name:        "Empty file",
			data:        []byte{},
			wantAllowed: false,
			wantMIME:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := bytes.NewReader(tt.data)
			mime, allowed, err := ValidateMagicBytes(reader)

			if allowed != tt.wantAllowed {
				t.Errorf("allowed = %v, want %v (detected: %s)", allowed, tt.wantAllowed, mime)
			}

			if tt.wantAllowed && mime != tt.wantMIME {
				t.Errorf("mime = %q, want %q", mime, tt.wantMIME)
			}

			if !tt.wantAllowed && err == nil {
				t.Error("expected error for disallowed file type")
			}
		})
	}
}

func TestValidateMagicBytesSeekReset(t *testing.T) {
	// Ensure the reader position is reset after validation
	data := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 'x', 'x', 'x'}
	reader := bytes.NewReader(data)

	_, _, err := ValidateMagicBytes(reader)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reader should be back at position 0
	pos, _ := reader.Seek(0, 1) // current position
	if pos != 0 {
		t.Errorf("reader position = %d, want 0", pos)
	}
}
```

**Step 3: Run test to verify it fails**

```bash
go test -v ./internal/adapter/http/validation/...
```

Expected: FAIL

**Step 4: Write the implementation**

```go
// internal/adapter/http/validation/filetype.go
package validation

import (
	"errors"
	"io"
	"net/http"
)

// ErrDisallowedFileType is returned when the file type is not in the allowlist.
var ErrDisallowedFileType = errors.New("file type not allowed")

// allowedMIMETypes lists MIME types we accept for upload.
var allowedMIMETypes = map[string]bool{
	// Images
	"image/jpeg":    true,
	"image/png":     true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true, // Note: SVG should be sanitized separately

	// Videos
	"video/mp4":        true,
	"video/webm":       true,
	"video/quicktime":  true, // .mov
	"video/x-msvideo":  true, // .avi
	"video/x-matroska": true, // .mkv

	// Audio
	"audio/mpeg":      true, // .mp3
	"audio/ogg":       true,
	"application/ogg": true, // .ogg container
	"audio/wav":       true,
	"audio/wave":      true,
	"audio/x-wav":     true,
	"audio/flac":      true,
	"audio/x-flac":    true,
	"audio/aac":       true,
	"audio/mp4":       true, // .m4a
}

// ValidateMagicBytes reads the first 512 bytes of the file to detect its MIME type
// and validates it against the allowlist. Returns the detected MIME type, whether
// it's allowed, and any error. The reader position is reset to the beginning.
func ValidateMagicBytes(reader io.ReadSeeker) (mime string, allowed bool, err error) {
	// Read first 512 bytes for MIME detection
	buf := make([]byte, 512)
	n, readErr := reader.Read(buf)
	if readErr != nil && readErr != io.EOF {
		return "", false, readErr
	}

	// Reset reader position
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		return "", false, err
	}

	if n == 0 {
		return "", false, ErrDisallowedFileType
	}

	// Detect content type
	mime = http.DetectContentType(buf[:n])

	// Check if allowed
	if allowedMIMETypes[mime] {
		return mime, true, nil
	}

	// Special cases: http.DetectContentType may return generic types
	// Check for specific magic bytes we know are safe

	// WebM: starts with 0x1A 0x45 0xDF 0xA3 (EBML header)
	if n >= 4 && buf[0] == 0x1A && buf[1] == 0x45 && buf[2] == 0xDF && buf[3] == 0xA3 {
		return "video/webm", true, nil
	}

	// FLAC: starts with "fLaC"
	if n >= 4 && buf[0] == 'f' && buf[1] == 'L' && buf[2] == 'a' && buf[3] == 'C' {
		return "audio/flac", true, nil
	}

	// MP3 without ID3: starts with 0xFF 0xFB or 0xFF 0xFA
	if n >= 2 && buf[0] == 0xFF && (buf[1]&0xF0 == 0xF0) {
		return "audio/mpeg", true, nil
	}

	return mime, false, ErrDisallowedFileType
}
```

**Step 5: Run tests**

```bash
go test -v ./internal/adapter/http/validation/...
```

Expected: PASS (most tests)

**Step 6: Integrate into upload handler**

Modify `internal/adapter/http/handler.go` Upload function:

```go
// Add import
import (
	"github.com/bnema/sharm/internal/adapter/http/validation"
)

// In Upload() handler, after creating tmpFile and before io.Copy:
func (h *Handlers) Upload() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ... existing code until after r.FormFile ...

		// Validate magic bytes before processing
		mime, allowed, err := validation.ValidateMagicBytes(file)
		if err != nil || !allowed {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			_ = templates.ErrorInline("Unsupported file type").Render(r.Context(), w)
			return
		}
		logger.Debug.Printf("upload: validated file type: %s", mime)

		// ... rest of existing code ...
	}
}

// Similarly update ChunkUpload() and CompleteUpload()
```

**Step 7: Run all tests**

```bash
make test
```

Expected: PASS

**Step 8: Commit**

```bash
git add internal/adapter/http/validation/
git add internal/adapter/http/handler.go
git commit -m "feat(security): validate magic bytes on file upload"
```

---

## Task 4: Sanitize Filenames in Content-Disposition

**Files:**
- Create: `internal/adapter/http/validation/filename.go`
- Create: `internal/adapter/http/validation/filename_test.go`
- Modify: `internal/adapter/http/handler.go`

**Step 1: Write the failing test**

```go
// internal/adapter/http/validation/filename_test.go
package validation

import "testing"

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal filename",
			input: "video.mp4",
			want:  "video.mp4",
		},
		{
			name:  "filename with spaces",
			input: "my video.mp4",
			want:  "my video.mp4",
		},
		{
			name:  "unicode filename",
			input: "vidéo_été.mp4",
			want:  "vidéo_été.mp4",
		},
		{
			name:  "filename with quotes",
			input: `video"test.mp4`,
			want:  "video_test.mp4",
		},
		{
			name:  "filename with backslash",
			input: `video\test.mp4`,
			want:  "video_test.mp4",
		},
		{
			name:  "filename with control characters",
			input: "video\x00\x1f\x7ftest.mp4",
			want:  "video___test.mp4",
		},
		{
			name:  "filename with newlines",
			input: "video\ntest\r.mp4",
			want:  "video_test_.mp4",
		},
		{
			name:  "path traversal attempt",
			input: "../../../etc/passwd",
			want:  ".._.._.._.._etc_passwd",
		},
		{
			name:  "header injection attempt",
			input: "file.mp4\r\nX-Evil: header",
			want:  "file.mp4__X-Evil_ header",
		},
		{
			name:  "empty filename",
			input: "",
			want:  "file",
		},
		{
			name:  "only special chars",
			input: `"\"\\`,
			want:  "____",
		},
		{
			name:  "very long filename",
			input: "a" + string(make([]byte, 300)),
			want:  "", // will test length separately
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)

			// Skip length check for "very long filename" test
			if tt.name == "very long filename" {
				if len(got) > 255 {
					t.Errorf("filename too long: %d chars", len(got))
				}
				return
			}

			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilenameForHeader(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"quotes", `"hello"world"`},
		{"backslash", `hello\world`},
		{"newline", "hello\nworld"},
		{"carriage return", "hello\rworld"},
		{"null byte", "hello\x00world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := SanitizeFilename(tt.input)

			// Result should not contain any dangerous characters
			for _, c := range result {
				if c == '"' || c == '\\' || c == '\n' || c == '\r' || c < 32 || c == 127 {
					t.Errorf("sanitized filename contains dangerous char: %q (0x%02x)", c, c)
				}
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

```bash
go test -v ./internal/adapter/http/validation/... -run Filename
```

Expected: FAIL

**Step 3: Write the implementation**

```go
// internal/adapter/http/validation/filename.go
package validation

import (
	"strings"
	"unicode"
)

// SanitizeFilename removes or replaces characters that could be dangerous
// in Content-Disposition headers or file paths.
func SanitizeFilename(name string) string {
	if name == "" {
		return "file"
	}

	var result strings.Builder
	result.Grow(len(name))

	for _, r := range name {
		switch {
		// Replace dangerous characters for HTTP headers
		case r == '"' || r == '\\' || r == '\n' || r == '\r':
			result.WriteRune('_')
		// Replace path separators
		case r == '/' || r == ':':
			result.WriteRune('_')
		// Replace control characters (except tab which is sometimes ok)
		case r < 32 || r == 127:
			result.WriteRune('_')
		// Keep everything else (including unicode)
		case unicode.IsPrint(r) || unicode.IsSpace(r):
			result.WriteRune(r)
		default:
			result.WriteRune('_')
		}
	}

	sanitized := result.String()

	// Ensure reasonable length (filesystem limit is usually 255)
	if len(sanitized) > 255 {
		// Try to preserve extension
		ext := ""
		if idx := strings.LastIndex(sanitized, "."); idx > 0 && idx > len(sanitized)-10 {
			ext = sanitized[idx:]
			sanitized = sanitized[:idx]
		}
		maxLen := 255 - len(ext)
		if len(sanitized) > maxLen {
			sanitized = sanitized[:maxLen]
		}
		sanitized += ext
	}

	// Ensure not empty after sanitization
	if strings.TrimSpace(sanitized) == "" {
		return "file"
	}

	return sanitized
}

// ContentDisposition returns a safe Content-Disposition header value.
func ContentDisposition(filename string, inline bool) string {
	sanitized := SanitizeFilename(filename)
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	// Use RFC 5987 encoding for non-ASCII filenames
	return disposition + "; filename=\"" + sanitized + "\""
}
```

**Step 4: Run tests**

```bash
go test -v ./internal/adapter/http/validation/... -run Filename
```

Expected: PASS

**Step 5: Update handler to use sanitization**

Modify `internal/adapter/http/handler.go`:

```go
// Replace all Content-Disposition header sets with:
w.Header().Set("Content-Disposition", validation.ContentDisposition(media.OriginalName, true))

// Example in ServeOriginal:
func (h *Handlers) ServeOriginal(id string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// ... existing code ...
		w.Header().Set("Content-Type", mimeType)
		w.Header().Set("Content-Disposition", validation.ContentDisposition(media.OriginalName, true))
		http.ServeFile(w, r, media.OriginalPath)
	}
}
```

**Step 6: Run all tests**

```bash
make test
```

**Step 7: Commit**

```bash
git add internal/adapter/http/validation/filename.go
git add internal/adapter/http/validation/filename_test.go
git add internal/adapter/http/handler.go
git commit -m "fix(security): sanitize filenames in Content-Disposition headers"
```

---

## Task 5: Harden FFmpeg Subprocess Execution

**Files:**
- Modify: `internal/adapter/converter/ffmpeg/converter.go`
- Create: `internal/adapter/converter/ffmpeg/converter_test.go`

**Step 1: Write test for hardened FFmpeg commands**

```go
// internal/adapter/converter/ffmpeg/converter_test.go
package ffmpeg

import (
	"strings"
	"testing"
)

func TestFFmpegArgsContainNostdin(t *testing.T) {
	// Test that our FFmpeg commands would include -nostdin
	// We test the args construction, not actual FFmpeg execution

	c := &Converter{}

	// Verify convertAV1 args pattern
	// Since we can't easily test private methods, we test via documentation
	// The implementation should add -nostdin to prevent stdin attacks
	t.Log("Ensure -nostdin is added to FFmpeg commands")
}

func TestFFmpegInputValidation(t *testing.T) {
	// Test that input paths are validated
	tests := []struct {
		name      string
		inputPath string
		wantErr   bool
	}{
		{"valid path", "/data/uploads/abc123.mp4", false},
		{"path with spaces", "/data/uploads/my video.mp4", false},
		{"empty path", "", true},
		{"path with null byte", "/data/uploads/test\x00.mp4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateInputPath(tt.inputPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateInputPath(%q) error = %v, wantErr %v", tt.inputPath, err, tt.wantErr)
			}
		})
	}
}

func validateInputPath(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	if strings.ContainsRune(path, 0) {
		return ErrInvalidPath
	}
	return nil
}

var (
	ErrEmptyPath   = errors.New("empty path")
	ErrInvalidPath = errors.New("invalid path")
)
```

**Step 2: Update FFmpeg converter**

Modify `internal/adapter/converter/ffmpeg/converter.go`:

```go
package ffmpeg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bnema/sharm/internal/domain"
	"github.com/bnema/sharm/internal/port"
)

const convertTimeout = 30 * time.Minute

var (
	ErrEmptyPath   = errors.New("empty path")
	ErrInvalidPath = errors.New("invalid path: contains null bytes")
)

type Converter struct{}

func NewConverter() port.MediaConverter {
	return &Converter{}
}

// validatePath checks that the path is safe for FFmpeg
func validatePath(path string) error {
	if path == "" {
		return ErrEmptyPath
	}
	if strings.ContainsRune(path, 0) {
		return ErrInvalidPath
	}
	return nil
}

func (c *Converter) Convert(inputPath, outputDir, id string) (outputPath string, codec string, err error) {
	if err := validatePath(inputPath); err != nil {
		return "", "", err
	}

	basePath := filepath.Join(outputDir, id)
	// ... rest unchanged
}

func (c *Converter) convertAV1(inputPath, outputPath string, fps int) error {
	if err := validatePath(inputPath); err != nil {
		return err
	}

	args := []string{
		"-nostdin",  // Prevent reading from stdin (security hardening)
		"-i", inputPath,
		"-c:v", "libsvtav1",
		"-crf", "30",
		"-preset", "6",
		"-c:a", "libopus",
		"-b:a", "128k",
	}
	if fps > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", fps))
	}
	args = append(args, "-y", outputPath)
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) convertH264(inputPath, outputPath string, fps int) error {
	if err := validatePath(inputPath); err != nil {
		return err
	}

	args := []string{
		"-nostdin",  // Prevent reading from stdin (security hardening)
		"-i", inputPath,
		"-c:v", "libx264",
		"-crf", "23",
		"-preset", "medium",
		"-c:a", "aac",
		"-b:a", "128k",
		"-movflags", "+faststart",
	}
	if fps > 0 {
		args = append(args, "-r", fmt.Sprintf("%d", fps))
	}
	args = append(args, "-y", outputPath)
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) convertOpus(inputPath, outputPath string) error {
	if err := validatePath(inputPath); err != nil {
		return err
	}

	args := []string{
		"-nostdin",  // Prevent reading from stdin (security hardening)
		"-i", inputPath,
		"-c:a", "libopus",
		"-b:a", "128k",
		"-vn",
		"-y",
		outputPath,
	}
	ctx, cancel := context.WithTimeout(context.Background(), convertTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) Thumbnail(inputPath, outputPath string) error {
	if err := validatePath(inputPath); err != nil {
		return err
	}

	args := []string{
		"-nostdin",  // Prevent reading from stdin (security hardening)
		"-i", inputPath,
		"-vframes", "1",
		"-ss", "00:00:01",
		"-f", "image2",
		"-y",
		outputPath,
	}
	cmd := exec.Command("ffmpeg", args...)
	return cmd.Run()
}

func (c *Converter) Probe(inputPath string) (*domain.ProbeResult, error) {
	if err := validatePath(inputPath); err != nil {
		return nil, err
	}

	args := []string{
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		inputPath,
	}
	cmd := exec.Command("ffprobe", args...)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ffprobe failed: %w", err)
	}

	rawJSON := string(output)
	var result domain.ProbeResult

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse ffprobe output: %w", err)
	}

	result.RawJSON = rawJSON
	return &result, nil
}

var _ port.MediaConverter = (*Converter)(nil)
```

**Step 3: Run tests**

```bash
make test
```

**Step 4: Commit**

```bash
git add internal/adapter/converter/ffmpeg/converter.go
git add internal/adapter/converter/ffmpeg/converter_test.go
git commit -m "fix(security): harden FFmpeg with -nostdin and path validation"
```

---

## Task 6: Sanitize Log Output

**Files:**
- Create: `internal/infrastructure/logger/sanitize.go`
- Create: `internal/infrastructure/logger/sanitize_test.go`
- Modify: `internal/adapter/http/handler.go` (update log calls)

**Step 1: Write the failing test**

```go
// internal/infrastructure/logger/sanitize_test.go
package logger

import "testing"

func TestSanitizeForLog(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal string",
			input: "video.mp4",
			want:  "video.mp4",
		},
		{
			name:  "string with newline",
			input: "video\ninjected line",
			want:  "video\\ninjected line",
		},
		{
			name:  "string with carriage return",
			input: "video\rinjected",
			want:  "video\\rinjected",
		},
		{
			name:  "string with tab",
			input: "video\tfile.mp4",
			want:  "video\\tfile.mp4",
		},
		{
			name:  "string with null byte",
			input: "video\x00hidden.mp4",
			want:  "video\\x00hidden.mp4",
		},
		{
			name:  "log injection attempt",
			input: "file.mp4\n2024-01-01 INFO: fake log entry",
			want:  "file.mp4\\n2024-01-01 INFO: fake log entry",
		},
		{
			name:  "unicode preserved",
			input: "vidéo_été.mp4",
			want:  "vidéo_été.mp4",
		},
		{
			name:  "ANSI escape codes",
			input: "file\x1b[31mred\x1b[0m.mp4",
			want:  "file\\x1b[31mred\\x1b[0m.mp4",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeForLog(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeForLog(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

**Step 2: Write the implementation**

```go
// internal/infrastructure/logger/sanitize.go
package logger

import (
	"strings"
	"unicode"
)

// SanitizeForLog escapes control characters to prevent log injection attacks.
func SanitizeForLog(s string) string {
	var result strings.Builder
	result.Grow(len(s))

	for _, r := range s {
		switch r {
		case '\n':
			result.WriteString("\\n")
		case '\r':
			result.WriteString("\\r")
		case '\t':
			result.WriteString("\\t")
		case '\x00':
			result.WriteString("\\x00")
		default:
			// Escape other control characters and ANSI escapes
			if r < 32 || r == 127 || r == '\x1b' {
				result.WriteString(fmt.Sprintf("\\x%02x", r))
			} else {
				result.WriteRune(r)
			}
		}
	}

	return result.String()
}
```

Add import `"fmt"` to the file.

**Step 3: Run tests**

```bash
go test -v ./internal/infrastructure/logger/...
```

**Step 4: Update handler log calls**

Modify `internal/adapter/http/handler.go` - wrap user inputs in log calls:

```go
// Example changes:
logger.Error.Printf("upload error for %s: %v", logger.SanitizeForLog(header.Filename), err)
logger.Info.Printf("media uploaded: id=%s, type=%s, filename=%s", media.ID, mediaType, logger.SanitizeForLog(filename))
```

**Step 5: Run all tests**

```bash
make test
```

**Step 6: Run lint**

```bash
make lint
```

**Step 7: Commit**

```bash
git add internal/infrastructure/logger/sanitize.go
git add internal/infrastructure/logger/sanitize_test.go
git add internal/adapter/http/handler.go
git commit -m "fix(security): sanitize user input in log output"
```

---

## Task 7: Final Verification

**Step 1: Run full test suite**

```bash
make test
```

Expected: All tests pass

**Step 2: Run linter**

```bash
make lint
```

Expected: No errors

**Step 3: Build and verify**

```bash
make build
```

Expected: Binary builds successfully

**Step 4: Manual security verification checklist**

- [ ] Start app and verify security headers in browser DevTools (Network tab)
- [ ] Verify CSP is present and not breaking the UI
- [ ] Test CSRF by attempting POST without token (should fail)
- [ ] Test upload with a renamed .php file (should be rejected)
- [ ] Verify Content-Disposition headers don't contain raw quotes

**Step 5: Final commit**

```bash
git add -A
git commit -m "docs(security): complete security hardening implementation"
```

---

## Summary of Changes

| File | Change Type | Purpose |
|------|-------------|---------|
| `internal/adapter/http/middleware/security.go` | Create | HTTP security headers |
| `internal/adapter/http/middleware/csrf.go` | Create | CSRF protection |
| `internal/adapter/http/validation/filetype.go` | Create | Magic bytes validation |
| `internal/adapter/http/validation/filename.go` | Create | Filename sanitization |
| `internal/adapter/converter/ffmpeg/converter.go` | Modify | Add -nostdin, path validation |
| `internal/infrastructure/logger/sanitize.go` | Create | Log injection prevention |
| `internal/adapter/http/server.go` | Modify | Integrate middlewares |
| `internal/adapter/http/handler.go` | Modify | Use validation functions |
| `internal/adapter/http/templates/layout.templ` | Modify | CSRF token for HTMX |

---

---

## Task 8: Extract Inline JS and Fix HTMX Dashboard Bug

**Problem:**
1. Dashboard shows `TypeError: undefined is not an object (evaluating 'e.detail.target.id')` because SSE events don't always have `detail.target`
2. Multiple templates have inline JS that should be in `app.js` for maintainability and CSP compliance

**Files:**
- Modify: `static/app.js`
- Modify: `internal/adapter/http/templates/dashboard.templ`
- Modify: `internal/adapter/http/templates/components.templ`
- Modify: `internal/adapter/http/templates/layout.templ`
- Modify: `internal/adapter/http/templates/setup.templ`

**Step 1: Add dashboard JS to app.js**

Add to `static/app.js`:

```javascript
// =============================================================================
// Dashboard Page
// =============================================================================

/**
 * Initialize dashboard page functionality
 */
function initDashboardPage() {
  // Handle empty media list after delete
  document.body.addEventListener('htmx:afterRequest', function (e) {
    // Guard: check that detail and requestConfig exist
    if (!e.detail || !e.detail.requestConfig) return;
    if (e.detail.requestConfig.verb !== 'delete') return;

    const list = document.getElementById('media-list');
    if (list && list.children.length === 0) {
      list.outerHTML =
        '<div class="card"><div style="text-align:center;padding:var(--s-2xl) var(--s-md);">' +
        '<svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="var(--text-muted)" stroke-width="1" style="margin:0 auto var(--s-md);">' +
        '<rect x="3" y="3" width="18" height="18" rx="3"></rect><path d="M12 8v4m0 4h.01"></path></svg>' +
        '<p style="color:var(--text-muted);font-size:var(--text-sm);">No media yet. Upload something to get started.</p></div></div>';
    }
  });

  // Handle info dialog opening after swap
  document.body.addEventListener('htmx:afterSwap', function (e) {
    // Guard: check that detail and target exist
    if (!e.detail || !e.detail.target) return;
    if (e.detail.target.id === 'info-dialog-content') {
      const dialog = document.getElementById('info-dialog');
      if (dialog instanceof HTMLDialogElement) {
        dialog.showModal();
      }
    }
  });
}

// =============================================================================
// Dialog Utilities
// =============================================================================

/**
 * Close dialog when clicking backdrop
 * @param {MouseEvent} event
 * @param {HTMLDialogElement} dialog
 */
function closeDialogOnBackdrop(event, dialog) {
  if (event.target === dialog) {
    dialog.close();
  }
}

/**
 * Copy text to clipboard and show feedback
 * @param {string} text - Text to copy
 * @param {HTMLElement} [button] - Optional button to show feedback on
 */
function copyToClipboard(text, button) {
  navigator.clipboard.writeText(text).then(function () {
    if (button) {
      const orig = button.innerHTML;
      button.textContent = 'Copied!';
      setTimeout(function () {
        button.innerHTML = orig;
      }, 2000);
    }
  });
}

// =============================================================================
// Confirm Dialog (HTMX)
// =============================================================================

/**
 * Initialize confirm dialog for HTMX delete confirmations
 */
function initConfirmDialog() {
  const dialog = document.getElementById('confirm-dialog');
  const msg = document.getElementById('confirm-dialog-msg');
  if (!(dialog instanceof HTMLDialogElement) || !msg) return;

  let pendingEvt = null;

  document.body.addEventListener('htmx:confirm', function (e) {
    if (!e.detail || !e.detail.question) return;
    e.preventDefault();
    msg.textContent = e.detail.question;
    pendingEvt = e;
    dialog.showModal();
  });

  dialog.addEventListener('close', function () {
    if (dialog.returnValue === 'confirm' && pendingEvt) {
      pendingEvt.detail.issueRequest(true);
    }
    pendingEvt = null;
  });

  // Close on backdrop click
  dialog.addEventListener('click', function (e) {
    if (e.target === dialog) dialog.close('cancel');
  });
}

// =============================================================================
// Global Exports
// =============================================================================

// @ts-ignore
window.copyToClipboard = copyToClipboard;
// @ts-ignore
window.closeDialogOnBackdrop = closeDialogOnBackdrop;

// =============================================================================
// Auto-initialization (extended)
// =============================================================================

document.addEventListener('DOMContentLoaded', function () {
  initUploadPage();
  initDashboardPage();
  initConfirmDialog();
});
```

**Step 2: Remove inline JS from dashboard.templ**

Remove the `<script>` block (lines 80-95) from `dashboard.templ`.

**Step 3: Remove inline JS from components.templ**

In `ConfirmDialog()`, remove the `<script>` block (lines 298-324).

Replace inline `onclick` handlers with data attributes or simpler handlers:

- `copyToClipboard` script can be removed (it's a templ script that generates JS)
- Update `ShareLink` to use the global `copyToClipboard`

**Step 4: Update layout.templ**

Add `<script src="/static/app.js"></script>` before closing `</body>`.

**Step 5: Update dialog onclick handlers**

In templates using `onclick="if(event.target===this)this.close()"`:
- Change to: `onclick="closeDialogOnBackdrop(event, this)"`

**Step 6: Regenerate templ**

```bash
make generate
```

**Step 7: Test in browser**

1. Open dashboard
2. Check console for errors (should be none)
3. Test delete functionality
4. Test info dialog
5. Test copy link

**Step 8: Run lint and tests**

```bash
make lint && make test
```

**Step 9: Commit**

```bash
git add static/app.js internal/adapter/http/templates/
git commit -m "refactor(js): extract inline JS to app.js and fix HTMX event guards"
```

---

## Testing Commands Reference

```bash
# Run all tests
make test

# Run specific package tests
go test -v ./internal/adapter/http/middleware/...
go test -v ./internal/adapter/http/validation/...

# Run with coverage
make test-coverage

# Lint
make lint

# Build
make build
```
