// Package dockerbox implements the sandbox runtime against the Docker Engine
// API. It is written against the API, not against any particular daemon, so the
// same code drives Docker Desktop, Colima, and OrbStack on macOS and a native
// daemon on Linux.
//
// Security posture for this slice (S2):
//
//   - The project directory is bind-mounted read-write at /workspace and is the
//     only host state the container can reach.
//   - `<project>/.protean` is masked: a shared, empty, read-only host directory
//     is mounted over /workspace/.protean, so the control files are neither
//     readable nor writable from inside, whatever the agent tries.
//   - `<project>/AGENTS.md` is bind-mounted read-only.
//   - CPU, memory (with no swap headroom), and pids are capped.
//   - Networking is disabled outright. The allowlist egress proxy is the next
//     slice (S3); until it exists the sandbox is default-closed rather than
//     open to the internet.
//   - The container runs as an explicit numeric non-root user, publishes no ports, and
//     is never auto-removed — this runtime owns its lifecycle.
//
// The remaining hardening flags from the design (cap-drop, no-new-privileges,
// read-only rootfs, size-limited tmpfs, internal network) land in S3 together
// with the proxy.
package dockerbox

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/client"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

const (
	// ContainerNamePrefix starts every container this runtime owns.
	ContainerNamePrefix = "protean-sb"

	// Labels make our containers discoverable after a backend restart, so a
	// crash does not orphan running sandboxes.
	LabelManaged = "com.protean.managed"
	LabelOrg     = "com.protean.org"
	LabelProject = "com.protean.project"

	// Resource defaults from the design: 0.25 vCPU, 1 GiB RAM with swap pinned
	// to the same value so there is no swap headroom, and 256 processes.
	DefaultNanoCPUs    = 250_000_000
	DefaultMemoryBytes = 1 << 30
	DefaultPidsLimit   = 256

	// MaxExecTimeout is the hard ceiling on any single command, whatever the
	// caller or the config asks for.
	MaxExecTimeout = 10 * time.Minute

	// MaxFileBytes bounds ReadFile so a large file cannot exhaust host memory.
	MaxFileBytes = 16 << 20

	// maskDirName is a single shared, empty, read-only directory mounted over
	// /workspace/.protean in every sandbox.
	maskDirName = "protean-sandbox-mask"

	// stopGracePeriod is how long a container gets to exit on SIGTERM before
	// the daemon kills it.
	stopGracePeriod = 5 * time.Second

	// sandboxUser is deliberately numeric so container execution never depends
	// on a mutable image's passwd database or default USER directive.
	sandboxUser = "10001:10001"
)

// refPattern keeps container names well-formed and unambiguous: ULIDs pass, and
// nothing can smuggle a separator or a shell character into a name.
var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// ErrImageMissing is returned when the sandbox image has not been built.
var ErrImageMissing = errors.New("sandbox image not found")

// Options configure the runtime. Zero values fall back to the package defaults.
type Options struct {
	// Image is the sandbox base image, e.g. "protean-sandbox:dev".
	Image string

	// DataDir is the platform data root; project directories live under
	// <DataDir>/protean-organizations/<org>/projects/<project>.
	DataDir string

	// ExecTimeout is the default per-command timeout, clamped by MaxExecTimeout.
	ExecTimeout time.Duration

	// IdleTimeout is how long a sandbox may sit unused before the reaper stops
	// it. ReapInterval is how often the reaper looks.
	IdleTimeout  time.Duration
	ReapInterval time.Duration

	// MaxRunning is the global cap on concurrently running sandboxes.
	MaxRunning int

	// OutputCapBytes caps stdout and stderr separately for one command.
	OutputCapBytes int

	// Container resource limits.
	NanoCPUs    int64
	MemoryBytes int64
	PidsLimit   int64

	Logger *slog.Logger
	Clock  ports.Clock
}

func (o Options) withDefaults() Options {
	if o.Image == "" {
		o.Image = "protean-sandbox:dev"
	}
	if o.DataDir == "" {
		o.DataDir = "."
	}
	if o.ExecTimeout <= 0 {
		o.ExecTimeout = 120 * time.Second
	}
	if o.ExecTimeout > MaxExecTimeout {
		o.ExecTimeout = MaxExecTimeout
	}
	if o.IdleTimeout <= 0 {
		o.IdleTimeout = 15 * time.Minute
	}
	if o.ReapInterval <= 0 {
		o.ReapInterval = time.Minute
	}
	if o.MaxRunning < 1 {
		o.MaxRunning = 4
	}
	if o.OutputCapBytes < 1 {
		o.OutputCapBytes = 256 << 10
	}
	if o.NanoCPUs <= 0 {
		o.NanoCPUs = DefaultNanoCPUs
	}
	if o.MemoryBytes <= 0 {
		o.MemoryBytes = DefaultMemoryBytes
	}
	if o.PidsLimit <= 0 {
		o.PidsLimit = DefaultPidsLimit
	}
	if o.Logger == nil {
		o.Logger = slog.Default()
	}
	return o
}

// Runtime implements ports.SandboxRuntime against one Docker daemon.
type Runtime struct {
	api   dockerAPI
	opts  Options
	log   *slog.Logger
	clock ports.Clock
	slots *slots

	mu     sync.Mutex
	states map[ports.ProjectRef]*state

	wg sync.WaitGroup
}

// state is one project's slice of runtime bookkeeping. opMu serializes the
// lifecycle operations for that project; mu (the Runtime's) guards the fields.
type state struct {
	opMu sync.Mutex

	containerID  string
	running      bool
	holdsSlot    bool
	lastActivity time.Time
	active       int

	// uid/gid of the container user, discovered once and reused so written
	// files land owned by the sandbox user rather than root.
	uid, gid int
	idsKnown bool
}

var _ ports.SandboxRuntime = (*Runtime)(nil)

// New builds a runtime on an existing Docker API client and adopts whatever
// labeled containers are already running, so a backend restart does not lose
// track of live sandboxes or of the concurrency budget they consume.
func New(ctx context.Context, api dockerAPI, opts Options) (*Runtime, error) {
	opts = opts.withDefaults()
	clk := opts.Clock
	if clk == nil {
		clk = systemClock{}
	}

	r := &Runtime{
		api:    api,
		opts:   opts,
		log:    opts.Logger,
		clock:  clk,
		slots:  newSlots(opts.MaxRunning),
		states: map[ports.ProjectRef]*state{},
	}
	if err := r.adoptRunning(ctx); err != nil {
		return nil, err
	}
	return r, nil
}

// adoptRunning finds containers this runtime owns that are already running and
// counts them against the concurrency cap.
func (r *Runtime) adoptRunning(ctx context.Context) error {
	list, err := r.api.ContainerList(ctx, container.ListOptions{
		Filters: filters.NewArgs(filters.Arg("label", LabelManaged+"=true")),
	})
	if err != nil {
		return fmt.Errorf("list protean sandboxes: %w", err)
	}

	now := r.clock.Now()
	for _, c := range list {
		ref := ports.ProjectRef{OrgID: c.Labels[LabelOrg], ProjectID: c.Labels[LabelProject]}
		if err := validateRef(ref); err != nil {
			r.log.Warn("sandbox: ignoring container with unusable labels", "container_id", c.ID, "err", err)
			continue
		}

		name := ContainerName(ref)
		insp, err := r.api.ContainerInspect(ctx, c.ID)
		if err != nil {
			return fmt.Errorf("inspect running sandbox %s: %w", name, err)
		}
		if err := r.validateContainer(ctx, ref, insp); err != nil {
			r.log.Warn("sandbox: replacing running container with invalid security configuration",
				"org_id", ref.OrgID, "project_id", ref.ProjectID, "container_id", c.ID, "err", err)
			if err := r.api.ContainerRemove(ctx, c.ID, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
				return fmt.Errorf("remove invalid sandbox %s: %w", name, err)
			}
			id, err := r.create(ctx, ref)
			if err != nil {
				return fmt.Errorf("recreate invalid sandbox %s: %w", name, err)
			}
			if err := r.api.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
				return fmt.Errorf("start recreated sandbox %s: %w", name, err)
			}
			insp.ID = id
		}

		st := r.stateFor(ref)
		r.mu.Lock()
		st.containerID = insp.ID
		st.running = true
		st.holdsSlot = true
		st.lastActivity = now
		r.mu.Unlock()
		r.slots.adopt()

		r.log.Info("sandbox: adopted running container",
			"org_id", ref.OrgID, "project_id", ref.ProjectID, "container_id", insp.ID)
	}
	return nil
}

func (r *Runtime) stateFor(ref ports.ProjectRef) *state {
	r.mu.Lock()
	defer r.mu.Unlock()
	st, ok := r.states[ref]
	if !ok {
		st = &state{}
		r.states[ref] = st
	}
	return st
}

// Ensure returns a running sandbox for ref.
func (r *Runtime) Ensure(ctx context.Context, ref ports.ProjectRef) (ports.Sandbox, error) {
	id, err := r.ensure(ctx, ref)
	if err != nil {
		return nil, err
	}
	return &sandboxHandle{rt: r, ref: ref, containerID: id}, nil
}

// ensure is the Ensure body shared with the Sandbox methods, which re-ensure
// before every operation so a handle survives the idle reaper.
func (r *Runtime) ensure(ctx context.Context, ref ports.ProjectRef) (string, error) {
	if err := validateRef(ref); err != nil {
		return "", err
	}

	st := r.stateFor(ref)
	st.opMu.Lock()
	defer st.opMu.Unlock()
	return r.ensureLocked(ctx, ref, st)
}

func (r *Runtime) ensureLocked(ctx context.Context, ref ports.ProjectRef, st *state) (string, error) {

	name := ContainerName(ref)
	insp, err := r.api.ContainerInspect(ctx, name)
	switch {
	case err == nil:
		if invariantErr := r.validateContainer(ctx, ref, insp); invariantErr != nil {
			r.log.Warn("sandbox: replacing container with invalid security configuration",
				"org_id", ref.OrgID, "project_id", ref.ProjectID, "container_id", insp.ID, "err", invariantErr)
			if err := r.api.ContainerRemove(ctx, insp.ID, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
				return "", fmt.Errorf("remove invalid sandbox %s: %w", name, err)
			}
			r.releaseSlot(st)
			id, err := r.create(ctx, ref)
			if err != nil {
				return "", err
			}
			if err := r.startExisting(ctx, ref, st, id); err != nil {
				return "", err
			}
			return id, nil
		}
		if insp.State != nil && insp.State.Running {
			r.noteRunning(st, insp.ID)
			return insp.ID, nil
		}
		if err := r.startExisting(ctx, ref, st, insp.ID); err != nil {
			return "", err
		}
		return insp.ID, nil

	case client.IsErrNotFound(err):
		id, err := r.create(ctx, ref)
		if err != nil {
			return "", err
		}
		if err := r.startExisting(ctx, ref, st, id); err != nil {
			return "", err
		}
		return id, nil

	default:
		return "", fmt.Errorf("inspect sandbox %s: %w", name, err)
	}
}

// beginOperation takes an activity lease before the reaper can stop the
// container. The lease lasts for the whole sandbox operation, not merely the
// Ensure call that precedes it.
func (r *Runtime) beginOperation(ctx context.Context, ref ports.ProjectRef) (string, func(), error) {
	if err := validateRef(ref); err != nil {
		return "", nil, err
	}
	st := r.stateFor(ref)
	st.opMu.Lock()
	r.mu.Lock()
	st.active++
	st.lastActivity = r.clock.Now()
	r.mu.Unlock()
	id, err := r.ensureLocked(ctx, ref, st)
	st.opMu.Unlock()
	if err != nil {
		r.finishOperation(st)
		return "", nil, err
	}
	var once sync.Once
	return id, func() { once.Do(func() { r.finishOperation(st) }) }, nil
}

func (r *Runtime) finishOperation(st *state) {
	r.mu.Lock()
	if st.active > 0 {
		st.active--
	}
	st.lastActivity = r.clock.Now()
	r.mu.Unlock()
}

// startExisting takes a concurrency slot, unless this project already holds
// one, and starts the container.
func (r *Runtime) startExisting(ctx context.Context, ref ports.ProjectRef, st *state, id string) error {
	// opMu serializes this project's lifecycle, so read-then-set is safe here.
	acquired := false
	if !r.holdsSlot(st) {
		if err := r.slots.acquire(ctx); err != nil {
			return fmt.Errorf("waiting for a free sandbox slot (%d of %d in use): %w",
				r.slots.inUse(), r.opts.MaxRunning, err)
		}
		r.setHoldsSlot(st, true)
		acquired = true
	}

	if err := r.api.ContainerStart(ctx, id, container.StartOptions{}); err != nil {
		if acquired {
			r.setHoldsSlot(st, false)
			r.slots.release()
		}
		return fmt.Errorf("start sandbox %s: %w", ContainerName(ref), err)
	}

	r.noteRunning(st, id)
	r.log.Info("sandbox: started",
		"org_id", ref.OrgID, "project_id", ref.ProjectID, "container_id", id, "running", r.slots.inUse())
	return nil
}

func (r *Runtime) holdsSlot(st *state) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return st.holdsSlot
}

func (r *Runtime) setHoldsSlot(st *state, v bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	st.holdsSlot = v
}

// noteRunning records a running container and refreshes its activity stamp. It
// adopts a slot when the project is not already counted — the case where a
// container was found running rather than started by us.
func (r *Runtime) noteRunning(st *state, id string) {
	r.mu.Lock()
	st.containerID = id
	st.running = true
	st.lastActivity = r.clock.Now()
	adopt := !st.holdsSlot
	st.holdsSlot = true
	r.mu.Unlock()

	if adopt {
		r.slots.adopt()
	}
}

// touch records activity, which is what keeps the idle reaper away.
func (r *Runtime) touch(ref ports.ProjectRef) {
	st := r.stateFor(ref)
	r.mu.Lock()
	defer r.mu.Unlock()
	st.lastActivity = r.clock.Now()
}

// create builds the container: host directories first, then the container.
func (r *Runtime) create(ctx context.Context, ref ports.ProjectRef) (string, error) {
	if _, err := r.api.ImageInspect(ctx, r.opts.Image); err != nil {
		if client.IsErrNotFound(err) {
			return "", fmt.Errorf("%w: %q is not present on the Docker daemon; build it with `make sandbox-image`: %w",
				ErrImageMissing, r.opts.Image, err)
		}
		return "", fmt.Errorf("inspect sandbox image %q: %w", r.opts.Image, err)
	}

	layout, err := r.prepareHost(ref)
	if err != nil {
		return "", err
	}

	cfg := &container.Config{
		Image:      r.opts.Image,
		User:       sandboxUser,
		WorkingDir: sandbox.WorkspaceRoot,
		Labels: map[string]string{
			LabelManaged: "true",
			LabelOrg:     ref.OrgID,
			LabelProject: ref.ProjectID,
		},
		// No egress at all until the S3 allowlist proxy exists.
		NetworkDisabled: true,
		Tty:             false,
	}

	pids := r.opts.PidsLimit
	initProcess := true
	hostCfg := &container.HostConfig{
		Mounts: []mount.Mount{
			{Type: mount.TypeBind, Source: layout.projectDir, Target: sandbox.WorkspaceRoot},
			{Type: mount.TypeBind, Source: layout.maskDir, Target: layout.controlTarget, ReadOnly: true},
			{Type: mount.TypeBind, Source: layout.agentsFile, Target: layout.agentsTarget, ReadOnly: true},
		},
		NetworkMode:   "none",
		AutoRemove:    false,
		Init:          &initProcess,
		RestartPolicy: container.RestartPolicy{Name: container.RestartPolicyDisabled},
		Resources: container.Resources{
			NanoCPUs:   r.opts.NanoCPUs,
			Memory:     r.opts.MemoryBytes,
			MemorySwap: r.opts.MemoryBytes,
			PidsLimit:  &pids,
		},
	}

	name := ContainerName(ref)
	created, err := r.api.ContainerCreate(ctx, cfg, hostCfg, nil, nil, name)
	if err != nil {
		return "", fmt.Errorf("create sandbox %s: %w", name, err)
	}
	r.log.Info("sandbox: created",
		"org_id", ref.OrgID, "project_id", ref.ProjectID, "container_id", created.ID, "image", r.opts.Image)
	return created.ID, nil
}

func (r *Runtime) validateContainer(ctx context.Context, ref ports.ProjectRef, insp container.InspectResponse) error {
	if insp.Name != "/"+ContainerName(ref) {
		return fmt.Errorf("canonical name is %q, want %q", insp.Name, "/"+ContainerName(ref))
	}
	if insp.Config == nil || insp.HostConfig == nil {
		return errors.New("container inspect omitted config")
	}
	labels := insp.Config.Labels
	if labels[LabelManaged] != "true" || labels[LabelOrg] != ref.OrgID || labels[LabelProject] != ref.ProjectID {
		return fmt.Errorf("project labels do not match %s/%s", ref.OrgID, ref.ProjectID)
	}
	imageInfo, err := r.api.ImageInspect(ctx, r.opts.Image)
	if err != nil {
		if client.IsErrNotFound(err) {
			return fmt.Errorf("%w: %q is not present; run `make sandbox-image`", ErrImageMissing, r.opts.Image)
		}
		return fmt.Errorf("inspect configured image %q: %w", r.opts.Image, err)
	}
	if insp.Config.Image != r.opts.Image || insp.Image != imageInfo.ID {
		return fmt.Errorf("image is %q (%s), want %q (%s)", insp.Config.Image, insp.Image, r.opts.Image, imageInfo.ID)
	}
	if insp.Config.User != sandboxUser {
		return fmt.Errorf("user is %q, want explicit non-root %q", insp.Config.User, sandboxUser)
	}
	if !insp.Config.NetworkDisabled || string(insp.HostConfig.NetworkMode) != "none" {
		return errors.New("networking is not disabled")
	}
	if insp.HostConfig.NanoCPUs != r.opts.NanoCPUs || insp.HostConfig.Memory != r.opts.MemoryBytes ||
		insp.HostConfig.MemorySwap != r.opts.MemoryBytes || insp.HostConfig.PidsLimit == nil ||
		*insp.HostConfig.PidsLimit != r.opts.PidsLimit {
		return errors.New("resource limits do not match runtime configuration")
	}
	if insp.HostConfig.RestartPolicy.Name != container.RestartPolicyDisabled || insp.HostConfig.RestartPolicy.MaximumRetryCount != 0 {
		return errors.New("restart policy is enabled")
	}
	if insp.HostConfig.AutoRemove {
		return errors.New("automatic removal is enabled")
	}
	if insp.HostConfig.Init == nil || !*insp.HostConfig.Init {
		return errors.New("container init process is not enabled")
	}

	layout, err := r.prepareHost(ref)
	if err != nil {
		return err
	}
	want := []mount.Mount{
		{Type: mount.TypeBind, Source: layout.projectDir, Target: sandbox.WorkspaceRoot},
		{Type: mount.TypeBind, Source: layout.maskDir, Target: layout.controlTarget, ReadOnly: true},
		{Type: mount.TypeBind, Source: layout.agentsFile, Target: layout.agentsTarget, ReadOnly: true},
	}
	if !sameMounts(insp.HostConfig.Mounts, want) {
		return errors.New("mounts do not exactly match the approved workspace, control mask, and AGENTS.md mounts")
	}
	return nil
}

func sameMounts(got, want []mount.Mount) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i].Type != want[i].Type || got[i].Source != want[i].Source || got[i].Target != want[i].Target ||
			got[i].ReadOnly != want[i].ReadOnly || got[i].BindOptions != nil || got[i].VolumeOptions != nil || got[i].TmpfsOptions != nil {
			return false
		}
	}
	return true
}

// hostLayout is the set of host paths one sandbox needs.
type hostLayout struct {
	projectDir    string
	maskDir       string
	agentsFile    string
	controlTarget string
	agentsTarget  string
}

// prepareHost creates the project directory, the read-only mask directory, and
// an AGENTS.md if the project has none, then resolves symlinks so the daemon
// sees the real paths (macOS temp dirs are symlinks into /private).
func (r *Runtime) prepareHost(ref ports.ProjectRef) (hostLayout, error) {
	projectDir := r.ProjectDir(ref)
	if err := os.MkdirAll(projectDir, layout.DirMode); err != nil {
		return hostLayout{}, fmt.Errorf("create project dir: %w", err)
	}
	if err := os.MkdirAll(layout.ProjectProteanDir(r.opts.DataDir, ref.OrgID, ref.ProjectID), layout.ControlDirMode); err != nil {
		return hostLayout{}, fmt.Errorf("create control dir: %w", err)
	}

	maskDir := filepath.Join(r.opts.DataDir, maskDirName)
	if err := os.MkdirAll(maskDir, 0o555); err != nil {
		return hostLayout{}, fmt.Errorf("create sandbox mask dir: %w", err)
	}

	resolvedProject, err := filepath.EvalSymlinks(projectDir)
	if err != nil {
		return hostLayout{}, fmt.Errorf("resolve project directory: %w", err)
	}
	agentsFile := layout.ProjectAgentsMD(r.opts.DataDir, ref.OrgID, ref.ProjectID)
	f, err := os.OpenFile(agentsFile, os.O_RDONLY|os.O_CREATE, 0o644)
	if err != nil {
		if err := os.Remove(agentsFile); err != nil {
			return hostLayout{}, fmt.Errorf("replace unsafe %s: %w", sandbox.AgentsFileName, err)
		}
		f, err = os.OpenFile(agentsFile, os.O_RDONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return hostLayout{}, fmt.Errorf("create safe %s: %w", sandbox.AgentsFileName, err)
		}
	}
	_ = f.Close()
	resolvedAgents, err := filepath.EvalSymlinks(agentsFile)
	if err != nil || !pathWithin(resolvedProject, resolvedAgents) || resolvedAgents != filepath.Join(resolvedProject, sandbox.AgentsFileName) {
		if removeErr := os.Remove(agentsFile); removeErr != nil {
			return hostLayout{}, fmt.Errorf("replace unsafe %s symlink: %w", sandbox.AgentsFileName, removeErr)
		}
		f, createErr := os.OpenFile(agentsFile, os.O_RDONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if createErr != nil {
			return hostLayout{}, fmt.Errorf("create safe %s: %w", sandbox.AgentsFileName, createErr)
		}
		_ = f.Close()
	}

	layout := hostLayout{
		projectDir:    projectDir,
		maskDir:       maskDir,
		agentsFile:    agentsFile,
		controlTarget: sandbox.WorkspaceRoot + "/" + sandbox.ControlDirName,
		agentsTarget:  sandbox.WorkspaceRoot + "/" + sandbox.AgentsFileName,
	}
	for _, p := range []*string{&layout.projectDir, &layout.maskDir, &layout.agentsFile} {
		resolved, err := filepath.EvalSymlinks(*p)
		if err != nil {
			return hostLayout{}, fmt.Errorf("resolve %q: %w", *p, err)
		}
		*p = resolved
	}
	return layout, nil
}

func pathWithin(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// ProjectDir is the host directory bind-mounted at the workspace root.
func (r *Runtime) ProjectDir(ref ports.ProjectRef) string {
	return layout.ProjectDir(r.opts.DataDir, ref.OrgID, ref.ProjectID)
}

// Stop stops the sandbox and frees its slot. Missing or already-stopped
// sandboxes succeed.
func (r *Runtime) Stop(ctx context.Context, ref ports.ProjectRef) error {
	if err := validateRef(ref); err != nil {
		return err
	}

	st := r.stateFor(ref)
	st.opMu.Lock()
	defer st.opMu.Unlock()
	return r.stopLocked(ctx, ref, st)
}

func (r *Runtime) stopLocked(ctx context.Context, ref ports.ProjectRef, st *state) error {
	name := ContainerName(ref)
	timeout := int(stopGracePeriod.Seconds())
	err := r.api.ContainerStop(ctx, name, container.StopOptions{Timeout: &timeout})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("stop sandbox %s: %w", name, err)
	}

	r.releaseSlot(st)
	r.log.Info("sandbox: stopped",
		"org_id", ref.OrgID, "project_id", ref.ProjectID, "running", r.slots.inUse())
	return nil
}

// replaceContainer force-removes a sandbox whose exec could not be proven
// dead, then recreates it from the canonical configuration. Project files are
// preserved because they live in the host bind mount.
func (r *Runtime) replaceContainer(ctx context.Context, ref ports.ProjectRef) error {
	st := r.stateFor(ref)
	st.opMu.Lock()
	defer st.opMu.Unlock()
	name := ContainerName(ref)
	if err := r.api.ContainerRemove(ctx, name, container.RemoveOptions{Force: true}); err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("force-remove sandbox %s: %w", name, err)
	}
	r.mu.Lock()
	st.running = false
	st.idsKnown = false
	r.mu.Unlock()
	id, err := r.create(ctx, ref)
	if err != nil {
		r.releaseSlot(st)
		return err
	}
	if err := r.startExisting(ctx, ref, st, id); err != nil {
		r.releaseSlot(st)
		return err
	}
	return nil
}

// Destroy removes the container. Project files on the host are untouched.
func (r *Runtime) Destroy(ctx context.Context, ref ports.ProjectRef) error {
	if err := validateRef(ref); err != nil {
		return err
	}

	st := r.stateFor(ref)
	st.opMu.Lock()
	defer st.opMu.Unlock()

	name := ContainerName(ref)
	err := r.api.ContainerRemove(ctx, name, container.RemoveOptions{Force: true})
	if err != nil && !client.IsErrNotFound(err) {
		return fmt.Errorf("remove sandbox %s: %w", name, err)
	}

	r.releaseSlot(st)
	r.mu.Lock()
	delete(r.states, ref)
	r.mu.Unlock()

	r.log.Info("sandbox: destroyed", "org_id", ref.OrgID, "project_id", ref.ProjectID)
	return nil
}

func (r *Runtime) releaseSlot(st *state) {
	r.mu.Lock()
	held := st.holdsSlot
	st.holdsSlot = false
	st.running = false
	st.idsKnown = false
	r.mu.Unlock()

	if held {
		r.slots.release()
	}
}

// Start runs the idle reaper until ctx is cancelled.
func (r *Runtime) Start(ctx context.Context) {
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(r.opts.ReapInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reap(ctx)
			}
		}
	}()
}

// Wait blocks until the reaper has stopped.
func (r *Runtime) Wait() { r.wg.Wait() }

// Close releases the daemon connection. Running sandboxes stay up and are
// adopted again on the next boot.
func (r *Runtime) Close() error { return r.api.Close() }

// reap stops every sandbox that has been idle longer than the idle timeout.
func (r *Runtime) reap(ctx context.Context) {
	now := r.clock.Now()

	r.mu.Lock()
	var idle []ports.ProjectRef
	for ref, st := range r.states {
		if st.running && now.Sub(st.lastActivity) >= r.opts.IdleTimeout {
			idle = append(idle, ref)
		}
	}
	r.mu.Unlock()

	for _, ref := range idle {
		if ctx.Err() != nil {
			return
		}
		st := r.stateFor(ref)
		st.opMu.Lock()
		r.mu.Lock()
		stillIdle := st.running && st.active == 0 && r.clock.Now().Sub(st.lastActivity) >= r.opts.IdleTimeout
		r.mu.Unlock()
		if !stillIdle {
			st.opMu.Unlock()
			continue
		}
		r.log.Info("sandbox: reaping idle sandbox",
			"org_id", ref.OrgID, "project_id", ref.ProjectID, "idle_timeout", r.opts.IdleTimeout)
		if err := r.stopLocked(ctx, ref, st); err != nil {
			r.log.Error("sandbox: reaping failed", "org_id", ref.OrgID, "project_id", ref.ProjectID, "err", err)
		}
		st.opMu.Unlock()
	}
}

// ContainerName is the deterministic container name for a project.
func ContainerName(ref ports.ProjectRef) string {
	return strings.Join([]string{ContainerNamePrefix, ref.OrgID, ref.ProjectID}, "-")
}

func validateRef(ref ports.ProjectRef) error {
	if !refPattern.MatchString(ref.OrgID) {
		return fmt.Errorf("sandbox: invalid org id %q", ref.OrgID)
	}
	if !refPattern.MatchString(ref.ProjectID) {
		return fmt.Errorf("sandbox: invalid project id %q", ref.ProjectID)
	}
	return nil
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }
