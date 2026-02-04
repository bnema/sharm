package middleware

import (
	"net/http"
	"strings"
)

// SecurityHeaders adds security-related HTTP headers to all responses.
// It sets X-Content-Type-Options, X-Frame-Options, Referrer-Policy,
// Permissions-Policy, Content-Security-Policy, and conditionally
// Strict-Transport-Security when behind TLS.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")

		// Control referrer information
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict browser features
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")

		// Content Security Policy
		csp := buildCSP()
		w.Header().Set("Content-Security-Policy", csp)

		// HTTP Strict Transport Security (only when behind TLS)
		if isTLS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next.ServeHTTP(w, r)
	})
}

// buildCSP constructs the Content-Security-Policy header value.
func buildCSP() string {
	directives := []string{
		"default-src 'self'",
		"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net",
		"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com",
		"font-src 'self' https://fonts.gstatic.com",
		"img-src 'self' data: blob:",
		"media-src 'self' blob:",
		"connect-src 'self'",
		"frame-ancestors 'none'",
	}
	return strings.Join(directives, "; ")
}

// isTLS checks if the request is served over TLS.
// It checks both the TLS connection state and the X-Forwarded-Proto header
// (for requests behind a reverse proxy).
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}
