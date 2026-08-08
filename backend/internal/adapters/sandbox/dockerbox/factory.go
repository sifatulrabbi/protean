package dockerbox

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
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
// environment, negotiates an API version, and verifies the image contract that
// every sandbox depends on.
func (f *Factory) Available(ctx context.Context) error {
	api, err := f.client()
	if err != nil {
		return err
	}
	if _, err := api.Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach the Docker daemon (is Docker Desktop, Colima, or OrbStack running?): %w", err)
	}
	info, err := api.ImageInspect(ctx, f.opts.Image)
	if err != nil {
		if client.IsErrNotFound(err) {
			return fmt.Errorf("%w: %q is not present on the Docker daemon; run `make sandbox-image`: %w",
				ErrImageMissing, f.opts.Image, err)
		}
		return fmt.Errorf("inspect sandbox image %q: %w", f.opts.Image, err)
	}
	if info.Config == nil || strings.TrimSpace(info.Config.User) == "" || info.Config.User == "0" ||
		info.Config.User == "root" || strings.HasPrefix(info.Config.User, "0:") || strings.HasPrefix(info.Config.User, "root:") {
		return fmt.Errorf("sandbox image %q has root or invalid USER %q; run `make sandbox-image`",
			f.opts.Image, func() string {
				if info.Config == nil {
					return ""
				}
				return info.Config.User
			}())
	}
	if err := f.probeImage(ctx, api); err != nil {
		return err
	}
	return nil
}

func (f *Factory) probeImage(ctx context.Context, api dockerAPI) (retErr error) {
	cfg := &container.Config{
		Image:           f.opts.Image,
		Cmd:             []string{"/bin/sh", "-c", `test "$(id -u)" -ne 0 && test -x /usr/bin/timeout`},
		NetworkDisabled: true,
	}
	created, err := api.ContainerCreate(ctx, cfg, &container.HostConfig{NetworkMode: "none"}, nil, nil, "")
	if err != nil {
		return fmt.Errorf("health-probe sandbox image %q (run `make sandbox-image`): create: %w", f.opts.Image, err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := api.ContainerRemove(cleanupCtx, created.ID, container.RemoveOptions{Force: true}); retErr == nil && err != nil && !client.IsErrNotFound(err) {
			retErr = fmt.Errorf("remove sandbox image health probe: %w", err)
		}
	}()
	if err := api.ContainerStart(ctx, created.ID, container.StartOptions{}); err != nil {
		return fmt.Errorf("health-probe sandbox image %q (run `make sandbox-image`): start: %w", f.opts.Image, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		insp, err := api.ContainerInspect(ctx, created.ID)
		if err != nil {
			return fmt.Errorf("health-probe sandbox image %q: inspect: %w", f.opts.Image, err)
		}
		if insp.State != nil && !insp.State.Running {
			if insp.State.ExitCode != 0 {
				return fmt.Errorf("sandbox image %q must run as non-root and contain executable /usr/bin/timeout; run `make sandbox-image`", f.opts.Image)
			}
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("health-probe sandbox image %q timed out; run `make sandbox-image`", f.opts.Image)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
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
