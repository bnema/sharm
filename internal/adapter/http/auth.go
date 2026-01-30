package http

import (
	"net/http"

	"github.com/bnema/sharm/internal/adapter/http/templates"
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
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		if err := authSvc.ValidateToken(cookie.Value); err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		next(w, r)
	}
}

func LoginHandler(authSvc AuthService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			renderLogin(w, r)
			return
		}

		if r.Method == http.MethodPost {
			password := r.FormValue("password")
			if !authSvc.ValidatePassword(password) {
				http.Redirect(w, r, "/login?error=1", http.StatusSeeOther)
				return
			}

			token := authSvc.GenerateToken()
			http.SetCookie(w, &http.Cookie{
				Name:     CookieName,
				Value:    token,
				MaxAge:   CookieMaxAge,
				Path:     CookiePath,
				Secure:   true,
				HttpOnly: true,
				SameSite: CookieSameSite,
			})

			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func renderLogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	templates.Login().Render(r.Context(), w)
}

func LogoutHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{
			Name:     CookieName,
			Value:    "",
			MaxAge:   -1,
			Path:     CookiePath,
			Secure:   true,
			HttpOnly: true,
			SameSite: CookieSameSite,
		})

		http.Redirect(w, r, "/login", http.StatusSeeOther)
	}
}
