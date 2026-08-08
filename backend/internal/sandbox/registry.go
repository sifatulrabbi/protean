package sandbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// Factory builds one sandbox runtime implementation. Adapters expose a Factory
// and are registered at boot, already carrying their configuration — the
// registry only decides which one runs.
type Factory interface {
	// Name identifies the adapter in logs and errors.
	Name() string

	// Platforms lists the GOOS values the adapter can serve. An empty list
	// means every platform.
	Platforms() []string

	// Available probes the adapter's prerequisites — the daemon socket, the
	// binary, the permissions. The returned error is shown to the operator, so
	// it must say what is missing and how to fix it.
	Available(ctx context.Context) error

	// New constructs the runtime. It is only called after Available returned
	// nil.
	New(ctx context.Context) (ports.SandboxRuntime, error)
}

// ErrNoRuntime is returned by Select when nothing healthy is available. It is a
// boot-stopping condition: there is no unsandboxed fallback, so the process
// must exit rather than run agent tools on the host.
var ErrNoRuntime = errors.New("no healthy sandbox runtime is available")

// Registry holds the candidate adapters and picks one at boot.
type Registry struct {
	logger    *slog.Logger
	goos      string
	factories []Factory
}

// NewRegistry returns an empty registry for the current platform.
func NewRegistry(logger *slog.Logger) *Registry {
	if logger == nil {
		logger = slog.Default()
	}
	return &Registry{logger: logger, goos: runtime.GOOS}
}

// Register appends a candidate. Candidates are tried in registration order, so
// the preferred adapter is registered first.
func (r *Registry) Register(f Factory) {
	if f == nil {
		return
	}
	r.factories = append(r.factories, f)
}

// Select returns the first registered adapter that supports this platform and
// passes its health probe. It fails with an error wrapping ErrNoRuntime when
// none does, listing why each candidate was skipped.
func (r *Registry) Select(ctx context.Context) (ports.SandboxRuntime, error) {
	var reasons []string

	for _, f := range r.factories {
		if !supportsPlatform(f, r.goos) {
			reasons = append(reasons, fmt.Sprintf("%s: does not support %s", f.Name(), r.goos))
			continue
		}
		if err := f.Available(ctx); err != nil {
			r.logger.Warn("sandbox: adapter unavailable", "adapter", f.Name(), "err", err)
			reasons = append(reasons, fmt.Sprintf("%s: %v", f.Name(), err))
			continue
		}

		rt, err := f.New(ctx)
		if err != nil {
			r.logger.Warn("sandbox: adapter failed to start", "adapter", f.Name(), "err", err)
			reasons = append(reasons, fmt.Sprintf("%s: %v", f.Name(), err))
			continue
		}

		r.logger.Info("sandbox: runtime selected", "adapter", f.Name(), "platform", r.goos)
		return rt, nil
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "no adapters registered")
	}
	return nil, fmt.Errorf("%w on %s: %s", ErrNoRuntime, r.goos, strings.Join(reasons, "; "))
}

func supportsPlatform(f Factory, goos string) bool {
	platforms := f.Platforms()
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == goos {
			return true
		}
	}
	return false
}
