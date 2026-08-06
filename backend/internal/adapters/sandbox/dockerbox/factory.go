package dockerbox

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/docker/docker/client"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
)

// Factory registers this adapter with the sandbox registry. It carries its
// configuration, so the registry only has to choose.
type Factory struct {
	opts Options
	log  *slog.Logger

	mu  sync.Mutex
	api dockerAPI
}

var _ sandbox.Factory = (*Factory)(nil)

// NewFactory returns the Docker adapter candidate.
func NewFactory(opts Options) *Factory {
	opts = opts.withDefaults()
	return &Factory{opts: opts, log: opts.Logger}
}

func (f *Factory) Name() string { return "docker" }

// Platforms: this adapter needs a reachable Docker daemon, which on macOS is
// Docker Desktop, Colima, or OrbStack, and on Linux the native daemon. Windows
// is out of scope for the platform.
func (f *Factory) Platforms() []string { return []string{"darwin", "linux"} }

// Available connects to the daemon named by the usual DOCKER_HOST/context
// environment, negotiates an API version, and pings it. A missing sandbox image
// is reported as a warning rather than a boot failure, because it is fixed with
// one make target and does not mean the runtime is unusable.
func (f *Factory) Available(ctx context.Context) error {
	api, err := f.client()
	if err != nil {
		return err
	}
	if _, err := api.Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach the Docker daemon (is Docker Desktop, Colima, or OrbStack running?): %w", err)
	}
	if _, err := api.ImageInspect(ctx, f.opts.Image); err != nil {
		f.log.Warn("sandbox: image not present yet, build it with `make sandbox-image`",
			"image", f.opts.Image, "err", err)
	}
	return nil
}

// New builds the runtime on the client Available already opened.
func (f *Factory) New(ctx context.Context) (ports.SandboxRuntime, error) {
	api, err := f.client()
	if err != nil {
		return nil, err
	}
	return New(ctx, api, f.opts)
}

func (f *Factory) client() (dockerAPI, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.api != nil {
		return f.api, nil
	}
	api, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("build Docker client: %w", err)
	}
	f.api = api
	return f.api, nil
}
