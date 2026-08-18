package http

import (
	"net"
	"net/http"

	"github.com/bnema/sharm/internal/adapter/http/middleware"
	"github.com/bnema/sharm/internal/adapter/http/ratelimit"
	"github.com/bnema/sharm/internal/adapter/http/templates"
	"github.com/bnema/sharm/internal/port"
	"github.com/bnema/sharm/internal/service"
	"github.com/bnema/sharm/static"
)

type Server struct {
	mux               *http.ServeMux
	handlers          *Handlers
	sseHandler        *SSEHandler
	authSvc           AuthService
	mediaSvc          MediaService
	rateLimiter       *ratelimit.LoginRateLimiter
	backoffTracker    *ratelimit.LoginAttemptTracker
	backoff           *ratelimit.Backoff
	csrf              *middleware.CSRFProtection
	behindProxy       bool
	trustedProxyCIDRs []*net.IPNet
	version           string
}

// ServerConfig holds all dependencies and settings needed to create an HTTP server.
type ServerConfig struct {
	AuthSvc           AuthService
	MediaSvc          MediaService
	ChunkSvc          *service.ChunkService
	EventBus          port.EventSubscriber
	Domain            string
	MaxUploadSizeMB   int
	Version           string
	BehindProxy       bool
	TrustedProxyCIDRs []*net.IPNet
	RateLimiter       *ratelimit.LoginRateLimiter
	BackoffTracker    *ratelimit.LoginAttemptTracker
	Backoff           *ratelimit.Backoff
	CSRF              *middleware.CSRFProtection
}

func NewServer(cfg ServerConfig) *Server {
	mux := http.NewServeMux()
	handlers := NewHandlers(cfg.MediaSvc, cfg.ChunkSvc, cfg.Domain, cfg.MaxUploadSizeMB, cfg.Version)
	sseHandler := NewSSEHandler(cfg.EventBus, cfg.MediaSvc, cfg.Domain)

	s := &Server{
		mux:               mux,
		handlers:          handlers,
		sseHandler:        sseHandler,
		authSvc:           cfg.AuthSvc,
		mediaSvc:          cfg.MediaSvc,
		rateLimiter:       cfg.RateLimiter,
		backoffTracker:    cfg.BackoffTracker,
		backoff:           cfg.Backoff,
		csrf:              cfg.CSRF,
		behindProxy:       cfg.BehindProxy,
		trustedProxyCIDRs: cfg.TrustedProxyCIDRs,
		version:           cfg.Version,
	}

	s.registerRoutes()
	s.registerStatic()

	return s
}

func (s *Server) registerRoutes() {
	setupHandler := SetupHandler(s.authSvc, s.version, s.behindProxy, s.trustedProxyCIDRs)
	s.mux.HandleFunc("GET /setup", setupHandler)
	s.mux.HandleFunc("POST /setup", setupHandler)

	loginHandler := LoginHandler(s.authSvc, s.rateLimiter, s.backoffTracker, s.backoff, s.version, s.behindProxy, s.trustedProxyCIDRs)
	s.mux.HandleFunc("GET /login", loginHandler)
	s.mux.HandleFunc("POST /login", loginHandler)

	s.mux.HandleFunc("POST /logout", AuthMiddleware(s.authSvc, LogoutHandler(s.authSvc, s.behindProxy)))

	s.mux.HandleFunc("POST /change-password", AuthMiddleware(s.authSvc, ChangePasswordHandler(s.authSvc)))

	s.mux.HandleFunc("GET /{$}", AuthMiddleware(s.authSvc, s.handlers.Dashboard()))

	s.mux.HandleFunc("GET /upload", AuthMiddleware(s.authSvc, s.handlers.UploadPage()))
	s.mux.HandleFunc("GET /config", AuthMiddleware(s.authSvc, s.handlers.ConfigPage()))

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
	// Chain: SecurityHeaders -> CSRF -> mux
	middleware.SecurityHeaders(s.csrf.Middleware(s.mux)).ServeHTTP(w, r)
}

// CSRFErrorHandler renders a CSRF error using templ components.
// For HTMX requests it returns an HTML error fragment with a script
// that updates the CSRF header from the fresh cookie. For plain
// requests it returns a 403 text response.
func CSRFErrorHandler(w http.ResponseWriter, r *http.Request, _ string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_ = templates.CSRFError().Render(r.Context(), w)
	} else {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}
}
