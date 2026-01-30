package http

import (
	"net/http"
	"time"

	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/service"
	"github.com/bnema/sharm/static"
)

type Server struct {
	mux            *http.ServeMux
	handlers       *Handlers
	authSvc        AuthService
	mediaSvc       MediaService
	rateLimiter    *ratelimit.LoginRateLimiter
	backoffTracker *ratelimit.LoginAttemptTracker
	backoff        *ratelimit.Backoff
}

func NewServer(authSvc *service.AuthService, mediaSvc MediaService, domain string, maxSizeMB int) *Server {
	mux := http.NewServeMux()
	handlers := NewHandlers(mediaSvc, domain, maxSizeMB)

	rateLimiter := ratelimit.NewLoginRateLimiter(
		5,
		15*time.Minute,
		30*time.Minute,
	)

	backoffTracker := ratelimit.NewLoginAttemptTracker()

	backoff := ratelimit.NewBackoff(
		500*time.Millisecond,
		10*time.Second,
		2.0,
	)

	s := &Server{
		mux:            mux,
		handlers:       handlers,
		authSvc:        authSvc,
		mediaSvc:       mediaSvc,
		rateLimiter:    rateLimiter,
		backoffTracker: backoffTracker,
		backoff:        backoff,
	}

	s.registerRoutes()
	s.registerStatic()

	return s
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.UploadPage())(w, r)
	})

	s.mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff)(w, r)
	})

	s.mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff)(w, r)
	})

	s.mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.Upload())(w, r)
	})

	s.mux.HandleFunc("GET /status/", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.Status())(w, r)
	})

	s.mux.HandleFunc("GET /v/", s.handlers.Media())
}

func (s *Server) registerStatic() {
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
