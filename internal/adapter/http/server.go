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
	sseHandler     *SSEHandler
	authSvc        AuthService
	mediaSvc       MediaService
	rateLimiter    *ratelimit.LoginRateLimiter
	backoffTracker *ratelimit.LoginAttemptTracker
	backoff        *ratelimit.Backoff
}

func NewServer(authSvc *service.AuthService, mediaSvc MediaService, eventBus *service.EventBus, domain string, maxSizeMB int) *Server {
	mux := http.NewServeMux()
	handlers := NewHandlers(mediaSvc, domain, maxSizeMB)
	sseHandler := NewSSEHandler(eventBus, mediaSvc, domain)

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
		sseHandler:     sseHandler,
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
	// Dashboard (library) is the root page
	s.mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.Dashboard())(w, r)
	})

	// Upload page
	s.mux.HandleFunc("GET /upload", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.UploadPage())(w, r)
	})

	// Login
	s.mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff)(w, r)
	})
	s.mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff)(w, r)
	})

	// Upload
	s.mux.HandleFunc("POST /upload", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.Upload())(w, r)
	})

	// Status page (full page for browser navigation)
	s.mux.HandleFunc("GET /status/", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.StatusPage())(w, r)
	})

	// SSE events (authenticated)
	s.mux.HandleFunc("GET /events/", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.sseHandler.Events())(w, r)
	})

	// Delete media (authenticated)
	s.mux.HandleFunc("DELETE /media/", func(w http.ResponseWriter, r *http.Request) {
		AuthMiddleware(s.authSvc, s.handlers.DeleteMedia())(w, r)
	})

	// Public share/raw/thumb
	s.mux.HandleFunc("GET /v/", s.handlers.Media())
}

func (s *Server) registerStatic() {
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
