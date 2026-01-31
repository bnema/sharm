package http

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/domain"
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

type contextKey string

const userKey contextKey = "user"

type AuthService interface {
	HasUser() (bool, error)
	ValidatePassword(username, password string) error
	GenerateToken(username string) (string, error)
	ValidateToken(token string) (*domain.User, error)
	CreateUser(username, password string) error
	ChangePassword(username, oldPassword, newPassword string) error
}

func AuthMiddleware(authSvc AuthService, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasUser, err := authSvc.HasUser()
		if err != nil {
			logger.Error.Printf("auth middleware: failed to check user existence: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasUser {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}

		cookie, err := r.Cookie(CookieName)
		if err != nil {
			logger.Debug.Printf("auth middleware: no cookie found, path=%s", r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		user, err := authSvc.ValidateToken(cookie.Value)
		if err != nil {
			logger.Warn.Printf("auth middleware: invalid token, error=%v, path=%s", err, r.URL.Path)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		ctx := context.WithValue(r.Context(), userKey, user)
		next(w, r.WithContext(ctx))
	}
}

func LoginHandler(authSvc AuthService, rateLimiter *ratelimit.LoginRateLimiter, tracker *ratelimit.LoginAttemptTracker, backoff *ratelimit.Backoff, version string, behindProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		clientID := getClientID(r)

		if r.Method == http.MethodGet {
			renderLogin(w, r, version)
			return
		}

		if r.Method == http.MethodPost {
			username := r.FormValue("username")
			password := r.FormValue("password")

			if username == "" || password == "" {
				logger.Info.Printf("login attempt: empty credentials from %s", clientID)
				renderFormError(w, r, "Username and password are required", http.StatusBadRequest)
				return
			}

			allowed, blockDuration := rateLimiter.Check(clientID)
			if !allowed {
				logger.Warn.Printf("login attempt: rate limit exceeded from %s, blocked for %v", clientID, blockDuration)
				w.Header().Set("Retry-After", fmt.Sprintf("%.0f", blockDuration.Seconds()))
				renderFormError(w, r, fmt.Sprintf("Too many attempts. Try again in %s", formatDuration(blockDuration)), http.StatusTooManyRequests)
				return
			}

			if err := authSvc.ValidatePassword(username, password); err != nil {
				tracker.RecordFailure(clientID)
				failedAttempts := tracker.GetFailedAttempts(clientID)

				backoffDuration := backoff.Duration(failedAttempts)
				if backoffDuration > 0 {
					logger.Info.Printf("login attempt: invalid credentials from %s (attempt %d), backing off for %v", clientID, failedAttempts, backoffDuration)
					time.Sleep(backoffDuration)
				} else {
					logger.Info.Printf("login attempt: invalid credentials from %s (attempt %d)", clientID, failedAttempts)
				}

				renderFormError(w, r, "Invalid username or password", http.StatusUnauthorized)
				return
			}

			tracker.RecordSuccess(clientID)
			rateLimiter.Reset(clientID)

			token, err := authSvc.GenerateToken(username)
			if err != nil {
				logger.Error.Printf("login: failed to generate token for %s: %v", username, err)
				renderFormError(w, r, "Internal error, please try again", http.StatusInternalServerError)
				return
			}

			setAuthCookie(w, r, token, behindProxy)
			logger.Info.Printf("login successful for %s from %s", username, clientID)

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

func renderLogin(w http.ResponseWriter, r *http.Request, version string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = templates.Login("", version).Render(r.Context(), w)
}

func LogoutHandler(behindProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secure := r.TLS != nil || behindProxy
		http.SetCookie(w, &http.Cookie{
			Name:     CookieName,
			Value:    "",
			MaxAge:   -1,
			Path:     CookiePath,
			Secure:   secure,
			HttpOnly: true,
			SameSite: CookieSameSite,
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}

func SetupHandler(authSvc AuthService, version string, behindProxy bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		hasUser, err := authSvc.HasUser()
		if err != nil {
			logger.Error.Printf("setup: failed to check user existence: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if hasUser {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if r.Method == http.MethodGet {
			renderSetup(w, r, version)
			return
		}

		if r.Method == http.MethodPost {
			username := r.FormValue("username")
			password := r.FormValue("password")
			confirmPassword := r.FormValue("confirm_password")

			if username == "" || password == "" {
				renderFormError(w, r, "Username and password are required", http.StatusBadRequest)
				return
			}

			if password != confirmPassword {
				renderFormError(w, r, "Passwords do not match", http.StatusBadRequest)
				return
			}

			if err := authSvc.CreateUser(username, password); err != nil {
				logger.Warn.Printf("setup: failed to create user: %v", err)
				renderFormError(w, r, "Failed to create user. Please try again.", http.StatusBadRequest)
				return
			}

			logger.Info.Printf("setup: user %s created successfully", username)

			token, err := authSvc.GenerateToken(username)
			if err != nil {
				logger.Error.Printf("setup: failed to generate token for %s: %v", username, err)
				renderFormError(w, r, "Account created but login failed. Please log in manually.", http.StatusInternalServerError)
				return
			}

			setAuthCookie(w, r, token, behindProxy)

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

func ChangePasswordHandler(authSvc AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, ok := r.Context().Value(userKey).(*domain.User)
		if !ok || user == nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		oldPassword := r.FormValue("old_password")
		newPassword := r.FormValue("new_password")
		confirmPassword := r.FormValue("confirm_password")

		if oldPassword == "" || newPassword == "" {
			renderFormError(w, r, "All fields are required", http.StatusBadRequest)
			return
		}

		if newPassword != confirmPassword {
			renderFormError(w, r, "New passwords do not match", http.StatusBadRequest)
			return
		}

		if err := authSvc.ChangePassword(user.Username, oldPassword, newPassword); err != nil {
			logger.Warn.Printf("change password: failed for user %s: %v", user.Username, err)
			renderFormError(w, r, "Failed to change password. Please verify your old password and try again.", http.StatusBadRequest)
			return
		}

		logger.Info.Printf("change password: successful for user %s", user.Username)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = templates.ChangePasswordSuccess().Render(r.Context(), w)
	}
}

func renderSetup(w http.ResponseWriter, r *http.Request, version string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = templates.Setup("", version).Render(r.Context(), w)
}

func renderFormError(w http.ResponseWriter, r *http.Request, msg string, status int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = templates.FormError(msg).Render(r.Context(), w)
}

func setAuthCookie(w http.ResponseWriter, r *http.Request, token string, behindProxy bool) {
	secure := r.TLS != nil || behindProxy
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		MaxAge:   CookieMaxAge,
		Path:     CookiePath,
		Secure:   secure,
		HttpOnly: true,
		SameSite: CookieSameSite,
	})
}
