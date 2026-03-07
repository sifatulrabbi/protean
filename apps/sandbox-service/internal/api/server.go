package api

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/protean/sandbox-service/internal/sandbox"
	"github.com/protean/sandbox-service/internal/security"
)

// Config bundles the dependencies needed to construct the HTTP server.
type Config struct {
	// ServiceTokens contains the accepted bearer tokens keyed by token value.
	ServiceTokens map[string]string
	// Service handles sandbox lifecycle and exec operations.
	Service *sandbox.Service
	// Logger records request failures and service events.
	Logger *slog.Logger
}

// Server exposes the sandbox service over HTTP.
type Server struct {
	// serviceTokens is the lookup table for bearer token authentication.
	serviceTokens map[string]string
	// service handles session and exec operations.
	service *sandbox.Service
	// locker serializes conflicting filesystem mutations within a workspace.
	locker *security.PathLocker
	// logger records request failures and denied operations.
	logger *slog.Logger
}

// New constructs the HTTP server using the caller-provided logger.
func New(config Config) *Server {
	if config.Logger == nil {
		panic("api.New requires Config.Logger")
	}

	return &Server{
		serviceTokens: config.ServiceTokens,
		service:       config.Service,
		locker:        security.NewPathLocker(),
		logger:        config.Logger,
	}
}

// ServeHTTP routes health, session, exec, and file requests.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	if !s.authorize(r) {
		s.logger.Warn("sandbox request unauthorized", "method", r.Method, "path", r.URL.Path)
		writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "invalid bearer token")
		return
	}

	switch {
	case r.URL.Path == "/api/v1/sandbox/sessions":
		s.handleSessions(w, r)
	case strings.HasPrefix(r.URL.Path, "/api/v1/sandbox/sessions/"):
		s.handleSessionRoute(w, r)
	default:
		s.logger.Warn("sandbox route not found", "method", r.Method, "path", r.URL.Path)
		http.NotFound(w, r)
	}
}

// writeMethodNotAllowed logs and returns the shared method-not-allowed payload.
func (s *Server) writeMethodNotAllowed(w http.ResponseWriter, r *http.Request, expected string) {
	s.logger.Warn(
		"sandbox method not allowed",
		"method", r.Method,
		"path", r.URL.Path,
		"expectedMethod", expected,
	)
	writeError(w, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
}
