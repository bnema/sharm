package http

import (
	"net/http"

	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/infrastructure/logger"
)

const (
	CookieName     = "auth_token"
	CookieMaxAge   = 7 * 24 * 60 * 60
	CookiePath     = "/"
	CookieSameSite = http.SameSiteStrictMode
)

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

func LoginHandler(authSvc AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			renderLogin(w, r, "")
			return
		}

		if r.Method == http.MethodPost {
			password := r.FormValue("password")
			if password == "" {
				logger.Info.Printf("login attempt: empty password from %s", r.RemoteAddr)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				templates.Login("Password is required").Render(r.Context(), w)
				return
			}

			if !authSvc.ValidatePassword(password) {
				logger.Info.Printf("login attempt: invalid password from %s", r.RemoteAddr)
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				w.WriteHeader(http.StatusUnauthorized)
				templates.Login("Invalid password").Render(r.Context(), w)
				return
			}

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

			logger.Info.Printf("login successful from %s", r.RemoteAddr)

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
