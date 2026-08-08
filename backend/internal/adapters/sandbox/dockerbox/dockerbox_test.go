package dockerbox

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
)

var testRef = ports.ProjectRef{OrgID: "01ORG", ProjectID: "01PROJ"}

// fakeClock lets the timing tests move time without sleeping. step, when set,
// advances the clock on every reading, which is how a test makes work inside a
// single call appear to take time.
type fakeClock struct {
	mu   sync.Mutex
	now  time.Time
	step time.Duration
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now
	c.now = c.now.Add(c.step)
	return now
}

func (c *fakeClock) setStep(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.step = d
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

type harness struct {
	rt    *Runtime
	api   *fakeDocker
	clock *fakeClock
}

func newHarness(t *testing.T, mutate func(*Options)) *harness {
	t.Helper()

	api := newFakeDocker()
	clk := &fakeClock{now: time.Date(2026, 3, 15, 10, 0, 0, 0, time.UTC)}
	dataDir := t.TempDir()

	opts := Options{
		Image:          "protean-sandbox:test",
		DataDir:        dataDir,
		ExecTimeout:    30 * time.Second,
		IdleTimeout:    15 * time.Minute,
		ReapInterval:   time.Millisecond,
		MaxRunning:     2,
		OutputCapBytes: 1024,
		Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		Clock:          clk,
	}
	if mutate != nil {
		mutate(&opts)
	}

	rt, err := New(context.Background(), api, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &harness{rt: rt, api: api, clock: clk}
}

func TestEnsureCreatesAndStartsContainer(t *testing.T) {
	h := newHarness(t, nil)

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if sb.Ref() != testRef {
		t.Errorf("Ref() = %+v, want %+v", sb.Ref(), testRef)
	}

	if len(h.api.creates) != 1 {
		t.Fatalf("ContainerCreate called %d times, want 1", len(h.api.creates))
	}
	call := h.api.creates[0]

	if want := "protean-sb-01ORG-01PROJ"; call.name != want {
		t.Errorf("container name = %q, want %q", call.name, want)
	}
	if got := call.config.Image; got != "protean-sandbox:test" {
		t.Errorf("image = %q, want protean-sandbox:test", got)
	}
	if got := call.config.WorkingDir; got != sandbox.WorkspaceRoot {
		t.Errorf("working dir = %q, want %q", got, sandbox.WorkspaceRoot)
	}
	if call.config.User != sandboxUser {
		t.Errorf("User = %q, want explicit non-root %q", call.config.User, sandboxUser)
	}

	// Labels keep the container discoverable after a backend restart.
	wantLabels := map[string]string{LabelManaged: "true", LabelOrg: "01ORG", LabelProject: "01PROJ"}
	for k, want := range wantLabels {
		if got := call.config.Labels[k]; got != want {
			t.Errorf("label %s = %q, want %q", k, got, want)
		}
	}

	// Network is off entirely until the S3 egress proxy exists.
	if !call.config.NetworkDisabled {
		t.Error("NetworkDisabled = false, want true")
	}
	if got := string(call.host.NetworkMode); got != "none" {
		t.Errorf("NetworkMode = %q, want none", got)
	}

	// Resource limits from the design.
	if got := call.host.Resources.NanoCPUs; got != DefaultNanoCPUs {
		t.Errorf("NanoCPUs = %d, want %d", got, int64(DefaultNanoCPUs))
	}
	if got := call.host.Resources.Memory; got != DefaultMemoryBytes {
		t.Errorf("Memory = %d, want %d", got, int64(DefaultMemoryBytes))
	}
	if got := call.host.Resources.MemorySwap; got != DefaultMemoryBytes {
		t.Errorf("MemorySwap = %d, want %d (no swap headroom)", got, int64(DefaultMemoryBytes))
	}
	if call.host.Resources.PidsLimit == nil || *call.host.Resources.PidsLimit != DefaultPidsLimit {
		t.Errorf("PidsLimit = %v, want %d", call.host.Resources.PidsLimit, DefaultPidsLimit)
	}
	if call.host.AutoRemove {
		t.Error("AutoRemove = true, want false: this runtime owns the lifecycle")
	}
	if call.host.Init == nil || !*call.host.Init {
		t.Error("Init is not enabled: orphaned exec descendants would not be reaped")
	}
	if len(call.host.PortBindings) != 0 {
		t.Errorf("PortBindings = %v, want none", call.host.PortBindings)
	}

	if len(h.api.snapshotStarted()) != 1 {
		t.Errorf("ContainerStart called %d times, want 1", len(h.api.snapshotStarted()))
	}
}

func TestEnsureMountsWorkspaceMaskAndAgentsFile(t *testing.T) {
	h := newHarness(t, nil)

	if _, err := h.rt.Ensure(context.Background(), testRef); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	mounts := h.api.creates[0].host.Mounts
	if len(mounts) != 3 {
		t.Fatalf("mounts = %d, want 3", len(mounts))
	}

	byTarget := map[string]struct {
		source   string
		readOnly bool
	}{}
	for _, m := range mounts {
		byTarget[m.Target] = struct {
			source   string
			readOnly bool
		}{m.Source, m.ReadOnly}
	}

	workspace, ok := byTarget[sandbox.WorkspaceRoot]
	if !ok {
		t.Fatalf("no mount at %s", sandbox.WorkspaceRoot)
	}
	if workspace.readOnly {
		t.Error("workspace mount is read-only, want read-write")
	}
	if !strings.HasSuffix(workspace.source, filepath.Join("protean-organizations", "01ORG", "projects", "01PROJ")) {
		t.Errorf("workspace source = %q, want the project directory", workspace.source)
	}

	mask, ok := byTarget["/workspace/.protean"]
	if !ok {
		t.Fatal("no mask mount at /workspace/.protean")
	}
	if !mask.readOnly {
		t.Error("mask mount is writable, want read-only")
	}
	entries, err := os.ReadDir(mask.source)
	if err != nil {
		t.Fatalf("read mask dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("mask dir holds %d entries, want an empty directory", len(entries))
	}
	if strings.HasPrefix(mask.source, workspace.source) {
		t.Errorf("mask source %q sits inside the project dir it masks", mask.source)
	}

	agents, ok := byTarget["/workspace/AGENTS.md"]
	if !ok {
		t.Fatal("no AGENTS.md mount")
	}
	if !agents.readOnly {
		t.Error("AGENTS.md mount is writable, want read-only: edits are approval-gated host-side")
	}
	if _, err := os.Stat(agents.source); err != nil {
		t.Errorf("AGENTS.md was not created host-side: %v", err)
	}
}

func TestEnsureReusesRunningContainer(t *testing.T) {
	h := newHarness(t, nil)
	first, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	h.api.creates = nil
	h.api.started = nil

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got, want := sb.(*sandboxHandle).containerID, first.(*sandboxHandle).containerID; got != want {
		t.Errorf("container id = %q, want %q", got, want)
	}
	if len(h.api.creates) != 0 {
		t.Errorf("created %d containers, want 0", len(h.api.creates))
	}
	if len(h.api.snapshotStarted()) != 0 {
		t.Errorf("started %d containers, want 0", len(h.api.snapshotStarted()))
	}
	if got := h.rt.slots.inUse(); got != 1 {
		t.Errorf("slots in use = %d, want 1", got)
	}
}

func TestEnsureStartsStoppedContainer(t *testing.T) {
	h := newHarness(t, nil)
	first, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("first Ensure: %v", err)
	}
	id := first.(*sandboxHandle).containerID
	if err := h.api.ContainerStop(context.Background(), id, container.StopOptions{}); err != nil {
		t.Fatalf("seed stopped container: %v", err)
	}
	h.rt.releaseSlot(h.rt.stateFor(testRef))
	h.api.creates = nil
	h.api.started = nil

	if _, err := h.rt.Ensure(context.Background(), testRef); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if len(h.api.creates) != 0 {
		t.Errorf("created %d containers, want 0", len(h.api.creates))
	}
	if !slices.Contains(h.api.snapshotStarted(), id) {
		t.Errorf("started = %v, want it to contain %s", h.api.snapshotStarted(), id)
	}
}

func TestEnsureReplacesContainersWithInvalidSecurityConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*container.InspectResponse)
	}{
		{"name", func(insp *container.InspectResponse) { insp.Name = "/colliding-container" }},
		{"labels", func(insp *container.InspectResponse) { insp.Config.Labels[LabelProject] = "OTHER" }},
		{"image reference", func(insp *container.InspectResponse) { insp.Config.Image = "attacker:latest" }},
		{"image id", func(insp *container.InspectResponse) { insp.Image = "sha256:stale" }},
		{"user", func(insp *container.InspectResponse) { insp.Config.User = "0" }},
		{"mount source", func(insp *container.InspectResponse) { insp.HostConfig.Mounts[0].Source = "/" }},
		{"mount readonly", func(insp *container.InspectResponse) { insp.HostConfig.Mounts[1].ReadOnly = false }},
		{"extra mount", func(insp *container.InspectResponse) {
			insp.HostConfig.Mounts = append(insp.HostConfig.Mounts, insp.HostConfig.Mounts[0])
		}},
		{"network", func(insp *container.InspectResponse) { insp.HostConfig.NetworkMode = "bridge" }},
		{"resources", func(insp *container.InspectResponse) { insp.HostConfig.Memory = 0 }},
		{"restart", func(insp *container.InspectResponse) { insp.HostConfig.RestartPolicy.Name = "always" }},
		{"init", func(insp *container.InspectResponse) { disabled := false; insp.HostConfig.Init = &disabled }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			first, err := h.rt.Ensure(context.Background(), testRef)
			if err != nil {
				t.Fatalf("first Ensure: %v", err)
			}
			oldID := first.(*sandboxHandle).containerID
			insp := h.api.containers[ContainerName(testRef)]
			tc.mutate(&insp)
			h.api.containers[ContainerName(testRef)] = insp
			h.api.creates = nil
			h.api.removed = nil

			if _, err := h.rt.Ensure(context.Background(), testRef); err != nil {
				t.Fatalf("Ensure after mutation: %v", err)
			}
			if !slices.Contains(h.api.removed, oldID) {
				t.Errorf("removed = %v, want invalid container %s destroyed", h.api.removed, oldID)
			}
			if len(h.api.creates) != 1 {
				t.Errorf("creates = %d, want one canonical replacement", len(h.api.creates))
			}
		})
	}
}

func TestPrepareHostReplacesAgentsSymlink(t *testing.T) {
	tests := []struct {
		name   string
		target func(*testing.T, *harness) string
	}{
		{"outside project", func(t *testing.T, h *harness) string {
			path := filepath.Join(t.TempDir(), "other-agents.md")
			if err := os.WriteFile(path, []byte("other project"), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}},
		{"system file", func(*testing.T, *harness) string { return "/etc/passwd" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, nil)
			project := h.rt.ProjectDir(testRef)
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			agents := filepath.Join(project, sandbox.AgentsFileName)
			if err := os.Symlink(tc.target(t, h), agents); err != nil {
				t.Fatal(err)
			}
			layout, err := h.rt.prepareHost(testRef)
			if err != nil {
				t.Fatalf("prepareHost: %v", err)
			}
			info, err := os.Lstat(layout.agentsFile)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Errorf("AGENTS.md remains a symlink: %s", layout.agentsFile)
			}
		})
	}
}

func TestEnsureMissingImageTellsOperatorHowToFixIt(t *testing.T) {
	h := newHarness(t, nil)
	h.api.imageMissing = true

	_, err := h.rt.Ensure(context.Background(), testRef)
	if !errors.Is(err, ErrImageMissing) {
		t.Fatalf("err = %v, want ErrImageMissing", err)
	}
	if !strings.Contains(err.Error(), "make sandbox-image") {
		t.Errorf("error %q does not tell the operator to run `make sandbox-image`", err)
	}
	if len(h.api.creates) != 0 {
		t.Error("a container was created despite the missing image")
	}
}

func TestFactoryAvailableRejectsUnusableImages(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeDocker)
		want   string
	}{
		{"missing", func(api *fakeDocker) { api.imageMissing = true }, "make sandbox-image"},
		{"empty user", func(api *fakeDocker) { api.imageUser = "" }, "root or invalid USER"},
		{"root user", func(api *fakeDocker) { api.imageUser = "0" }, "root or invalid USER"},
		{"probe failure", func(api *fakeDocker) { api.healthExit = 1 }, "/usr/bin/timeout"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			api := newFakeDocker()
			tc.mutate(api)
			factory := NewFactory(Options{Image: "protean-sandbox:test", Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
			factory.api = api
			err := factory.Available(context.Background())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Available error = %v, want it to contain %q", err, tc.want)
			}
		})
	}
}

func TestEnsureRejectsUnusableRefs(t *testing.T) {
	h := newHarness(t, nil)

	for _, ref := range []ports.ProjectRef{
		{OrgID: "", ProjectID: "01PROJ"},
		{OrgID: "01ORG", ProjectID: ""},
		{OrgID: "01ORG", ProjectID: "../escape"},
		{OrgID: "-leading-dash", ProjectID: "01PROJ"},
		{OrgID: "01ORG", ProjectID: "has space"},
	} {
		if _, err := h.rt.Ensure(context.Background(), ref); err == nil {
			t.Errorf("Ensure(%+v) succeeded, want an error", ref)
		}
	}
}

func TestAdoptsRunningContainersOnBoot(t *testing.T) {
	api := newFakeDocker()
	dataDir := t.TempDir()
	opts := Options{DataDir: dataDir, MaxRunning: 2, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	seed, err := New(context.Background(), api, opts)
	if err != nil {
		t.Fatalf("seed runtime: %v", err)
	}
	refs := []ports.ProjectRef{{OrgID: "01ORG", ProjectID: "01A"}, {OrgID: "01ORG", ProjectID: "01B"}}
	for _, ref := range refs {
		if _, err := seed.Ensure(context.Background(), ref); err != nil {
			t.Fatalf("seed %v: %v", ref, err)
		}
	}
	api.list = []container.Summary{
		{ID: seed.stateFor(refs[0]).containerID, Labels: map[string]string{LabelManaged: "true", LabelOrg: "01ORG", LabelProject: "01A"}},
		{ID: seed.stateFor(refs[1]).containerID, Labels: map[string]string{LabelManaged: "true", LabelOrg: "01ORG", LabelProject: "01B"}},
		{ID: "id-bad", Labels: map[string]string{LabelManaged: "true"}},
	}

	rt, err := New(context.Background(), api, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := rt.slots.inUse(); got != 2 {
		t.Errorf("slots in use = %d, want 2 (the unlabelled container is ignored)", got)
	}
}

func TestAdoptionReplacesInvalidRunningContainer(t *testing.T) {
	api := newFakeDocker()
	opts := Options{DataDir: t.TempDir(), MaxRunning: 1, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	seed, err := New(context.Background(), api, opts)
	if err != nil {
		t.Fatal(err)
	}
	sb, err := seed.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatal(err)
	}
	oldID := sb.(*sandboxHandle).containerID
	insp := api.containers[ContainerName(testRef)]
	insp.HostConfig.Mounts[0].Source = "/"
	api.containers[ContainerName(testRef)] = insp
	api.list = []container.Summary{{ID: oldID, Labels: map[string]string{
		LabelManaged: "true", LabelOrg: testRef.OrgID, LabelProject: testRef.ProjectID,
	}}}
	api.creates = nil
	api.removed = nil

	rt, err := New(context.Background(), api, opts)
	if err != nil {
		t.Fatalf("restart New: %v", err)
	}
	if !slices.Contains(api.removed, oldID) || len(api.creates) != 1 {
		t.Fatalf("adoption removed=%v creates=%d, want invalid container replaced", api.removed, len(api.creates))
	}
	if got := rt.slots.inUse(); got != 1 {
		t.Errorf("slots in use = %d, want replacement adopted once", got)
	}
}

func TestEnsureQueuesBeyondTheConcurrencyCap(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.MaxRunning = 1 })
	ctx := context.Background()

	first := ports.ProjectRef{OrgID: "01ORG", ProjectID: "01A"}
	second := ports.ProjectRef{OrgID: "01ORG", ProjectID: "01B"}

	if _, err := h.rt.Ensure(ctx, first); err != nil {
		t.Fatalf("Ensure first: %v", err)
	}

	// The cap is full, so the second Ensure must wait rather than start.
	blocked, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	if _, err := h.rt.Ensure(blocked, second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Ensure err = %v, want DeadlineExceeded", err)
	}

	// Stopping the first frees the slot and lets the second through.
	if err := h.rt.Stop(ctx, first); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if _, err := h.rt.Ensure(ctx, second); err != nil {
		t.Fatalf("second Ensure after Stop: %v", err)
	}
	if got := h.rt.slots.inUse(); got != 1 {
		t.Errorf("slots in use = %d, want 1", got)
	}
}

func TestStopAndDestroyFreeTheSlot(t *testing.T) {
	h := newHarness(t, nil)
	ctx := context.Background()

	if _, err := h.rt.Ensure(ctx, testRef); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if got := h.rt.slots.inUse(); got != 1 {
		t.Fatalf("slots in use = %d, want 1", got)
	}

	if err := h.rt.Stop(ctx, testRef); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := h.rt.slots.inUse(); got != 0 {
		t.Errorf("slots in use after Stop = %d, want 0", got)
	}
	// Stop is idempotent.
	if err := h.rt.Stop(ctx, testRef); err != nil {
		t.Errorf("second Stop: %v", err)
	}

	if _, err := h.rt.Ensure(ctx, testRef); err != nil {
		t.Fatalf("Ensure after Stop: %v", err)
	}
	if err := h.rt.Destroy(ctx, testRef); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if got := h.rt.slots.inUse(); got != 0 {
		t.Errorf("slots in use after Destroy = %d, want 0", got)
	}
	if !slices.Contains(h.api.removed, ContainerName(testRef)) {
		t.Errorf("removed = %v, want the sandbox container", h.api.removed)
	}
	// Destroying something that is gone is not an error.
	if err := h.rt.Destroy(ctx, testRef); err != nil {
		t.Errorf("second Destroy: %v", err)
	}
}

func TestReaperStopsIdleSandboxesOnly(t *testing.T) {
	h := newHarness(t, func(o *Options) {
		o.IdleTimeout = 10 * time.Minute
		o.ReapInterval = time.Millisecond
	})
	ctx := context.Background()

	busy := ports.ProjectRef{OrgID: "01ORG", ProjectID: "01BUSY"}
	idle := ports.ProjectRef{OrgID: "01ORG", ProjectID: "01IDLE"}
	if _, err := h.rt.Ensure(ctx, busy); err != nil {
		t.Fatalf("Ensure busy: %v", err)
	}
	if _, err := h.rt.Ensure(ctx, idle); err != nil {
		t.Fatalf("Ensure idle: %v", err)
	}

	reaperCtx, stop := context.WithCancel(ctx)
	h.rt.Start(reaperCtx)
	defer func() {
		stop()
		h.rt.Wait()
	}()

	h.clock.advance(11 * time.Minute)
	h.rt.touch(busy)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if slices.Contains(h.api.snapshotStopped(), ContainerName(idle)) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle sandbox was not reaped; stopped = %v", h.api.snapshotStopped())
		}
		time.Sleep(time.Millisecond)
	}
	if slices.Contains(h.api.snapshotStopped(), ContainerName(busy)) {
		t.Error("the busy sandbox was reaped")
	}
}

func TestReaperDoesNotStopActiveOperationAndCompletionRefreshesActivity(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.IdleTimeout = time.Minute })
	_, finish, err := h.rt.beginOperation(context.Background(), testRef)
	if err != nil {
		t.Fatalf("begin operation: %v", err)
	}
	h.clock.advance(2 * time.Minute)
	h.rt.reap(context.Background())
	if slices.Contains(h.api.snapshotStopped(), ContainerName(testRef)) {
		t.Fatal("reaper stopped a sandbox with an active operation")
	}
	finish()
	h.rt.reap(context.Background())
	if slices.Contains(h.api.snapshotStopped(), ContainerName(testRef)) {
		t.Fatal("reaper ignored the completion activity stamp")
	}
	h.clock.advance(2 * time.Minute)
	h.rt.reap(context.Background())
	if !slices.Contains(h.api.snapshotStopped(), ContainerName(testRef)) {
		t.Fatal("sandbox was not reaped after becoming idle")
	}
}

func TestExecPlumbing(t *testing.T) {
	h := newHarness(t, nil)
	h.api.script(execScript{stdout: []byte("hello\n"), stderr: []byte("warn\n"), exitCode: 3})

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	res, err := sb.Exec(context.Background(), ports.ExecSpec{
		Command: "echo hello",
		Cwd:     "src",
		Env:     []string{"FOO=bar"},
		Timeout: 45 * time.Second,
	})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if res.Stdout != "hello\n" || res.Stderr != "warn\n" {
		t.Errorf("stdout/stderr = %q/%q, want the demultiplexed streams", res.Stdout, res.Stderr)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if res.Truncated || res.TimedOut {
		t.Errorf("truncated = %v, timed out = %v, want both false", res.Truncated, res.TimedOut)
	}

	execs := h.api.snapshotExecs()
	if len(execs) != 1 {
		t.Fatalf("execs = %d, want 1", len(execs))
	}
	opts := execs[0].opts
	if len(opts.Cmd) != 8 || opts.Cmd[0] != "/usr/bin/timeout" || opts.Cmd[2] != "45s" || opts.Cmd[7] != "echo hello" {
		t.Errorf("Cmd = %q, want absolute timeout and the original command", opts.Cmd)
	}
	if opts.WorkingDir != "/workspace/src" {
		t.Errorf("WorkingDir = %q, want /workspace/src", opts.WorkingDir)
	}
	if !slices.Equal(opts.Env, []string{"FOO=bar", "PATH=/usr/local/bin:/usr/bin:/bin"}) {
		t.Errorf("Env = %q, want caller value plus canonical PATH", opts.Env)
	}
	if !opts.AttachStdout || !opts.AttachStderr {
		t.Error("stdout/stderr are not attached")
	}
	if opts.Privileged || opts.Tty {
		t.Errorf("Privileged = %v, Tty = %v, want both false", opts.Privileged, opts.Tty)
	}
}

func TestExecSanitizesSupervisorEnvironment(t *testing.T) {
	h := newHarness(t, nil)
	h.api.script(execScript{})
	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	_, err = sb.Exec(context.Background(), ports.ExecSpec{Command: "true", Env: []string{
		"PATH=/workspace", "LD_PRELOAD=/workspace/evil.so", "BASH_ENV=/workspace/evil", "SAFE=value",
	}})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	want := []string{"SAFE=value", "PATH=/usr/local/bin:/usr/bin:/bin"}
	if got := h.api.snapshotExecs()[0].opts.Env; !slices.Equal(got, want) {
		t.Errorf("Env = %q, want %q", got, want)
	}
}

func TestExecTimeoutBounds(t *testing.T) {
	tests := []struct {
		name       string
		configured time.Duration
		requested  time.Duration
		want       string
	}{
		{"zero uses the configured default", 30 * time.Second, 0, "30s"},
		{"caller value is honoured", 30 * time.Second, 90 * time.Second, "90s"},
		{"above the ceiling is clamped", 30 * time.Second, time.Hour, "600s"},
		{"sub-second is raised to a second", 30 * time.Second, 100 * time.Millisecond, "1s"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, func(o *Options) { o.ExecTimeout = tc.configured })
			h.api.script(execScript{})

			sb, err := h.rt.Ensure(context.Background(), testRef)
			if err != nil {
				t.Fatalf("Ensure: %v", err)
			}
			if _, err := sb.Exec(context.Background(), ports.ExecSpec{Command: "true", Timeout: tc.requested}); err != nil {
				t.Fatalf("Exec: %v", err)
			}

			cmd := h.api.snapshotExecs()[0].opts.Cmd
			if cmd[2] != tc.want {
				t.Errorf("timeout argument = %q, want %q (full argv %q)", cmd[2], tc.want, cmd)
			}
		})
	}
}

func TestExecCapsOutput(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.OutputCapBytes = 16 })
	h.api.script(execScript{stdout: bytes.Repeat([]byte("a"), 100), stderr: []byte("short")})

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	res, err := sb.Exec(context.Background(), ports.ExecSpec{Command: "yes"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}

	if len(res.Stdout) != 16 {
		t.Errorf("stdout length = %d, want 16", len(res.Stdout))
	}
	if res.Stderr != "short" {
		t.Errorf("stderr = %q, want it kept whole", res.Stderr)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
}

func TestExecTimedOutFlag(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.ExecTimeout = time.Second })
	h.api.script(execScript{exitCode: exitTimedOut})

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	// The fake returns instantly, so make the clock tick during the call and
	// the elapsed time look like the timeout really expired.
	h.clock.setStep(2 * time.Second)

	res, err := sb.Exec(context.Background(), ports.ExecSpec{Command: "sleep 60"})
	if err != nil {
		t.Fatalf("Exec: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true for exit code 124 after the timeout elapsed")
	}
}

func TestExecRejectsBadCwd(t *testing.T) {
	h := newHarness(t, nil)
	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	for _, cwd := range []string{"/etc", "../..", ".protean", "sub/.protean"} {
		if _, err := sb.Exec(context.Background(), ports.ExecSpec{Command: "ls", Cwd: cwd}); err == nil {
			t.Errorf("Exec with cwd %q succeeded, want a rejection", cwd)
		}
	}
	if n := len(h.api.snapshotExecs()); n != 0 {
		t.Errorf("%d execs reached the daemon, want 0", n)
	}
}

func TestFileOpsRejectUnsafePaths(t *testing.T) {
	h := newHarness(t, nil)
	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	ctx := context.Background()

	for _, p := range []string{"", "/etc/passwd", "../outside", ".protean/threads/x.json", ".PROTEAN/x"} {
		if err := sb.WriteFile(ctx, p, []byte("x"), 0o644); err == nil {
			t.Errorf("WriteFile(%q) succeeded, want a rejection", p)
		}
		if _, err := sb.ReadFile(ctx, p); err == nil {
			t.Errorf("ReadFile(%q) succeeded, want a rejection", p)
		}
		if err := sb.DeleteFile(ctx, p); err == nil {
			t.Errorf("DeleteFile(%q) succeeded, want a rejection", p)
		}
	}
	if len(h.api.copiedTo) != 0 {
		t.Errorf("%d copies reached the daemon, want 0", len(h.api.copiedTo))
	}
}

func TestWriteFileUsesAnchoredExec(t *testing.T) {
	h := newHarness(t, nil)
	h.api.script(execScript{})

	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	if err := sb.WriteFile(context.Background(), "src/pkg/main.go", []byte("package main"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if len(h.api.copiedTo) != 0 {
		t.Fatalf("copies = %d, want 0: Docker tar extraction is symlink-raceable", len(h.api.copiedTo))
	}
	execs := h.api.snapshotExecs()
	if len(execs) != 1 {
		t.Fatalf("execs = %d, want 1", len(execs))
	}
	if !execs[0].opts.AttachStdin {
		t.Error("secure writer did not attach stdin")
	}
	command := execs[0].opts.Cmd[6]
	if !strings.Contains(command, `actual=$(/usr/bin/readlink -f /proc/self/fd/3)`) ||
		!strings.Contains(command, `[ "$actual" = "$parent" ]`) || !strings.Contains(command, "/bin/mv -fT") {
		t.Errorf("secure writer command does not anchor and verify the parent directory: %q", command)
	}
}

func TestReadFile(t *testing.T) {
	h := newHarness(t, nil)
	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	ctx := context.Background()

	h.api.copyFrom["/workspace/notes.md"] = tarballOf(t, "notes.md", []byte("hello"))

	got, err := sb.ReadFile(ctx, "notes.md")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("contents = %q, want hello", got)
	}

	if _, err := sb.ReadFile(ctx, "missing.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile(missing) err = %v, want fs.ErrNotExist", err)
	}
}

func TestDeleteFile(t *testing.T) {
	h := newHarness(t, nil)
	sb, err := h.rt.Ensure(context.Background(), testRef)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}

	h.api.script(execScript{exitCode: 0})
	if err := sb.DeleteFile(context.Background(), "src/old.go"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	cmd := h.api.snapshotExecs()[0].opts.Cmd
	if !strings.Contains(cmd[7], "rm -rf -- '/workspace/src/old.go'") {
		t.Errorf("delete command = %q, does not remove the resolved path", cmd[7])
	}

	h.api.script(execScript{exitCode: missingPathExit})
	err = sb.DeleteFile(context.Background(), "gone.go")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("DeleteFile(missing) err = %v, want fs.ErrNotExist", err)
	}
}

func TestShellQuote(t *testing.T) {
	tests := []struct{ in, want string }{
		{"/workspace/a.txt", `'/workspace/a.txt'`},
		{"/workspace/it's here", `'/workspace/it'\''s here'`},
		{"/workspace/$(rm -rf /)", `'/workspace/$(rm -rf /)'`},
	}
	for _, tc := range tests {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func tarballOf(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len(data))}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write header: %v", err)
	}
	if _, err := tw.Write(data); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close archive: %v", err)
	}
	return buf.Bytes()
}
