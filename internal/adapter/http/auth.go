package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/infrastructure/logger"
)

const (
	CookieName     = "auth_token"
	CookieMaxAge   = 7 * 24 * 60 * 60
	CookiePath     = "/"
	CookieSameSite = http.SameSiteStrictMode
)

func getClientID(r *http.Request) string {
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		return forwarded
	}
	return r.RemoteAddr
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	}
	return fmt.Sprintf("%d hours", int(d.Hours()))
}

type AuthService interface {
	ValidatePassword(password string) bool
	GenerateToken() string
	ValidateToken(token string) error
}

func AuthMiddleware(authSvc AuthService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			logger.Debug.Printf("auth middleware: no cookie found, path=%s", r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if err := authSvc.ValidateToken(cookie.Value); err != nil {
			logger.Warn.Printf("auth middleware: invalid token, error=%v, path=%s", err, r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

func LoginHandler(authSvc AuthService, rateLimiter *ratelimit.LoginRateLimiter, tracker *ratelimit.LoginAttemptTracker, backoff *ratelimit.Backoff) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := getClientID(r)

		if r.Method == http.MethodGet {
			renderLogin(w, r, "")
			return
		}

		if r.Method == http.MethodPost {
			password := r.FormValue("password")
			if password == "" {
				logger.Info.Printf("login attempt: empty password from %s", clientID)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				templates.Login("Password is required").Render(r.Context(), w)
				return
			}

			allowed, blockDuration := rateLimiter.Check(clientID)
			if !allowed {
				logger.Warn.Printf("login attempt: rate limit exceeded from %s, blocked for %v", clientID, blockDuration)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", blockDuration.Seconds()))
				w.WriteHeader(http.StatusTooManyRequests)
				templates.Login(fmt.Sprintf("Too many attempts. Try again in %s", formatDuration(blockDuration))).Render(r.Context(), w)
				return
			}

			if !authSvc.ValidatePassword(password) {
				tracker.RecordFailure(clientID)
				failedAttempts := tracker.GetFailedAttempts(clientID)

				backoffDuration := backoff.Duration(failedAttempts)
				if backoffDuration > 0 {
					logger.Info.Printf("login attempt: invalid password from %s (attempt %d), backing off for %v", clientID, failedAttempts, backoffDuration)
					time.Sleep(backoffDuration)
				} else {
					logger.Info.Printf("login attempt: invalid password from %s (attempt %d)", clientID, failedAttempts)
				}

				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				templates.Login("Invalid password").Render(r.Context(), w)
				return
			}

			tracker.RecordSuccess(clientID)
			rateLimiter.Reset(clientID)

			token := authSvc.GenerateToken()
			http.SetCookie(w, &http.Cookie{
				Name:     CookieName,
				Value:    token,
				MaxAge:   CookieMaxAge,
				Path:     CookiePath,
				Secure:   r.TLS != nil,
				HttpOnly: true,
				SameSite: CookieSameSite,
			})

			logger.Info.Printf("login successful from %s", clientID)

			if r.Header.Get("HX-Request") == "true" {
				w.Header().Set("HX-Redirect", "/")
				return
			}

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func renderLogin(w http.ResponseWriter, r *http.Request, errorMsg string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	templates.Login(errorMsg).Render(r.Context(), w)
}

func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     CookieName,
			Value:    "",
			MaxAge:   -1,
			Path:     CookiePath,
			Secure:   r.TLS != nil,
			HttpOnly: true,
			SameSite: CookieSameSite,
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
