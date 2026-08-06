// Package httpserver wires HTTP transport to injected dependencies.
package httpserver

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

type Deps struct {
	Addr   string
	Logger *slog.Logger
	Clock  ports.Clock
}

type Server struct {
	http   *http.Server
	logger *slog.Logger
}

func New(deps Deps) *Server {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	h := &handlers{clock: deps.Clock}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.healthz)

	return &Server{
		http: &http.Server{
			Addr:              deps.Addr,
			Handler:           requestLogger(logger)(mux),
			ReadHeaderTimeout: 10 * time.Second,
		},
		logger: logger,
	}
}

// Handler exposes the routed handler for tests.
func (s *Server) Handler() http.Handler { return s.http.Handler }

func (s *Server) Addr() string { return s.http.Addr }

// Run serves until ctx is cancelled, then shuts down gracefully.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("http server listening", "addr", s.http.Addr)
		if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		s.logger.Info("http server shutting down")
		if err := s.http.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}
