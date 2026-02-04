package http

import (
	"net/http"
	"time"

	"github.com/bnema/sharm/internal/adapter/http/middleware"
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
	behindProxy    bool
	version        string
}

func NewServer(authSvc AuthService, mediaSvc MediaService, eventBus *service.EventBus, domain string, maxSizeMB int, version string, behindProxy bool) *Server {
	mux := http.NewServeMux()
	handlers := NewHandlers(mediaSvc, domain, maxSizeMB, version)
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
		behindProxy:    behindProxy,
		version:        version,
	}

	s.registerRoutes()
	s.registerStatic()

	return s
}

func (s *Server) registerRoutes() {
	setupHandler := SetupHandler(s.authSvc, s.version, s.behindProxy)
	s.mux.HandleFunc("GET /setup", setupHandler)
	s.mux.HandleFunc("POST /setup", setupHandler)

	loginHandler := LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff, s.version, s.behindProxy)
	s.mux.HandleFunc("GET /login", loginHandler)
	s.mux.HandleFunc("POST /login", loginHandler)

	s.mux.HandleFunc("POST /logout", AuthMiddleware(s.authSvc, LogoutHandler(s.behindProxy)))

	s.mux.HandleFunc("POST /change-password", AuthMiddleware(s.authSvc, ChangePasswordHandler(s.authSvc)))

	s.mux.HandleFunc("GET /{$}", AuthMiddleware(s.authSvc, s.handlers.Dashboard()))

	s.mux.HandleFunc("GET /upload", AuthMiddleware(s.authSvc, s.handlers.UploadPage()))

	s.mux.HandleFunc("POST /upload", AuthMiddleware(s.authSvc, s.handlers.Upload()))
	s.mux.HandleFunc("POST /upload/chunk", AuthMiddleware(s.authSvc, s.handlers.ChunkUpload()))
	s.mux.HandleFunc("POST /upload/complete", AuthMiddleware(s.authSvc, s.handlers.CompleteUpload()))

	s.mux.HandleFunc("GET /status/", AuthMiddleware(s.authSvc, s.handlers.StatusPage()))

	s.mux.HandleFunc("GET /events/", AuthMiddleware(s.authSvc, s.sseHandler.Events()))

	s.mux.HandleFunc("DELETE /media/", AuthMiddleware(s.authSvc, s.handlers.DeleteMedia()))

	s.mux.HandleFunc("GET /media/", AuthMiddleware(s.authSvc, s.handlers.MediaInfo()))

	s.mux.HandleFunc("GET /v/", s.handlers.Media())
}

func (s *Server) registerStatic() {
	s.mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(static.FS))))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	middleware.SecurityHeaders(s.mux).ServeHTTP(w, r)
}
