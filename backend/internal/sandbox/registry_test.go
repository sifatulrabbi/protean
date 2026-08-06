package sandbox

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// fakeFactory is a candidate whose probe and constructor are scripted.
type fakeFactory struct {
	name      string
	platforms []string
	available error
	newErr    error

	probed int
	built  int
}

func (f *fakeFactory) Name() string        { return f.name }
func (f *fakeFactory) Platforms() []string { return f.platforms }

func (f *fakeFactory) Available(context.Context) error {
	f.probed++
	return f.available
}

func (f *fakeFactory) New(context.Context) (ports.SandboxRuntime, error) {
	f.built++
	if f.newErr != nil {
		return nil, f.newErr
	}
	return &fakeRuntime{name: f.name}, nil
}

type fakeRuntime struct{ name string }

func (r *fakeRuntime) Ensure(context.Context, ports.ProjectRef) (ports.Sandbox, error) {
	return nil, errors.New("not implemented")
}
func (r *fakeRuntime) Stop(context.Context, ports.ProjectRef) error    { return nil }
func (r *fakeRuntime) Destroy(context.Context, ports.ProjectRef) error { return nil }
func (r *fakeRuntime) Start(context.Context)                           {}
func (r *fakeRuntime) Wait()                                           {}
func (r *fakeRuntime) Close() error                                    { return nil }

func newTestRegistry(t *testing.T, goos string) *Registry {
	t.Helper()
	r := NewRegistry(slog.New(slog.NewTextHandler(io.Discard, nil)))
	r.goos = goos
	return r
}

func TestSelect(t *testing.T) {
	t.Run("picks the first healthy candidate in registration order", func(t *testing.T) {
		first := &fakeFactory{name: "first", platforms: []string{"darwin"}}
		second := &fakeFactory{name: "second", platforms: []string{"darwin"}}

		reg := newTestRegistry(t, "darwin")
		reg.Register(first)
		reg.Register(second)

		rt, err := reg.Select(context.Background())
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if got := rt.(*fakeRuntime).name; got != "first" {
			t.Errorf("selected %q, want %q", got, "first")
		}
		if second.probed != 0 {
			t.Errorf("second candidate probed %d times, want 0", second.probed)
		}
	})

	t.Run("skips candidates for other platforms", func(t *testing.T) {
		linuxOnly := &fakeFactory{name: "linuxonly", platforms: []string{"linux"}}
		anywhere := &fakeFactory{name: "anywhere"}

		reg := newTestRegistry(t, "darwin")
		reg.Register(linuxOnly)
		reg.Register(anywhere)

		rt, err := reg.Select(context.Background())
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if got := rt.(*fakeRuntime).name; got != "anywhere" {
			t.Errorf("selected %q, want %q", got, "anywhere")
		}
		if linuxOnly.probed != 0 {
			t.Errorf("linux-only candidate was probed on darwin")
		}
	})

	t.Run("falls through to the next candidate when the probe fails", func(t *testing.T) {
		unhealthy := &fakeFactory{name: "unhealthy", available: errors.New("daemon down")}
		healthy := &fakeFactory{name: "healthy"}

		reg := newTestRegistry(t, "darwin")
		reg.Register(unhealthy)
		reg.Register(healthy)

		rt, err := reg.Select(context.Background())
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if got := rt.(*fakeRuntime).name; got != "healthy" {
			t.Errorf("selected %q, want %q", got, "healthy")
		}
	})

	t.Run("falls through when construction fails", func(t *testing.T) {
		broken := &fakeFactory{name: "broken", newErr: errors.New("cannot list containers")}
		healthy := &fakeFactory{name: "healthy"}

		reg := newTestRegistry(t, "linux")
		reg.Register(broken)
		reg.Register(healthy)

		rt, err := reg.Select(context.Background())
		if err != nil {
			t.Fatalf("Select: %v", err)
		}
		if got := rt.(*fakeRuntime).name; got != "healthy" {
			t.Errorf("selected %q, want %q", got, "healthy")
		}
	})

	// The boot-failure path: without a healthy runtime the process must not
	// come up, because there is no unsandboxed fallback.
	t.Run("fails the boot when nothing is registered", func(t *testing.T) {
		reg := newTestRegistry(t, "darwin")

		_, err := reg.Select(context.Background())
		if !errors.Is(err, ErrNoRuntime) {
			t.Fatalf("err = %v, want ErrNoRuntime", err)
		}
		if !strings.Contains(err.Error(), "no adapters registered") {
			t.Errorf("error %q does not explain that nothing was registered", err)
		}
	})

	t.Run("fails the boot when every candidate is unusable, listing why", func(t *testing.T) {
		reg := newTestRegistry(t, "darwin")
		reg.Register(&fakeFactory{name: "docker", available: errors.New("daemon down")})
		reg.Register(&fakeFactory{name: "applecontainer", platforms: []string{"linux"}})

		_, err := reg.Select(context.Background())
		if !errors.Is(err, ErrNoRuntime) {
			t.Fatalf("err = %v, want ErrNoRuntime", err)
		}
		for _, want := range []string{"docker: daemon down", "applecontainer: does not support darwin"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not contain %q", err, want)
			}
		}
	})

	t.Run("ignores a nil registration", func(t *testing.T) {
		reg := newTestRegistry(t, "darwin")
		reg.Register(nil)
		if _, err := reg.Select(context.Background()); !errors.Is(err, ErrNoRuntime) {
			t.Fatalf("err = %v, want ErrNoRuntime", err)
		}
	})
}
