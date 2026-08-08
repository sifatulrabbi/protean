package dockerbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
)

// These tests drive a real Docker daemon. They are opt-in because CI does not
// always have one:
//
//	PROTEAN_TEST_DOCKER=1 go test ./internal/adapters/sandbox/... -run Integration -v
//
// PROTEAN_TEST_SANDBOX_IMAGE overrides the image; the default expects
// `make sandbox-image` to have run.

const integrationEnv = "PROTEAN_TEST_DOCKER"

type integrationEnvT struct {
	rt  *Runtime
	api dockerAPI
	ref ports.ProjectRef
	dir string
}

func newIntegration(t *testing.T, mutate func(*Options)) *integrationEnvT {
	t.Helper()
	if os.Getenv(integrationEnv) != "1" {
		t.Skipf("set %s=1 to run the Docker integration tests", integrationEnv)
	}

	image := os.Getenv("PROTEAN_TEST_SANDBOX_IMAGE")
	if image == "" {
		image = "protean-sandbox:dev"
	}

	api, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		t.Fatalf("docker client: %v", err)
	}
	ctx := context.Background()
	if _, err := api.Ping(ctx); err != nil {
		t.Fatalf("docker daemon is not reachable: %v", err)
	}
	if _, err := api.ImageInspect(ctx, image); err != nil {
		t.Fatalf("sandbox image %q is missing; run `make sandbox-image`: %v", image, err)
	}

	dir := t.TempDir()
	opts := Options{
		Image:          image,
		DataDir:        dir,
		ExecTimeout:    30 * time.Second,
		IdleTimeout:    time.Hour,
		ReapInterval:   time.Hour,
		MaxRunning:     2,
		OutputCapBytes: 4096,
		Logger:         slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	}
	if mutate != nil {
		mutate(&opts)
	}

	rt, err := New(ctx, api, opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// A unique ref per test keeps parallel runs and leftovers apart.
	ref := ports.ProjectRef{
		OrgID:     "01TESTORG",
		ProjectID: strings.ToUpper(strings.ReplaceAll(t.Name(), "/", "")) + "-" + time.Now().Format("150405.000"),
	}
	ref.ProjectID = strings.ReplaceAll(ref.ProjectID, ".", "")
	if len(ref.ProjectID) > 64 {
		ref.ProjectID = ref.ProjectID[:64]
	}

	env := &integrationEnvT{rt: rt, api: api, ref: ref, dir: dir}
	// Teardown runs even when the test fails, so no container is left behind.
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := rt.Destroy(cleanupCtx, ref); err != nil {
			t.Logf("teardown: destroy %s: %v", ContainerName(ref), err)
		}
		_ = rt.Close()
		// Docker Desktop can release bind mounts just after ContainerRemove
		// returns. Remove the temporary data root with a short retry so
		// testing.TempDir cleanup does not race that unmount.
		deadline := time.Now().Add(3 * time.Second)
		for {
			err := os.RemoveAll(dir)
			if err == nil || time.Now().After(deadline) {
				if err != nil {
					t.Logf("teardown: remove temporary data root: %v", err)
				}
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
	return env
}

func (e *integrationEnvT) ensure(t *testing.T) ports.Sandbox {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sb, err := e.rt.Ensure(ctx, e.ref)
	if err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	return sb
}

func (e *integrationEnvT) exec(t *testing.T, sb ports.Sandbox, spec ports.ExecSpec) ports.ExecResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := sb.Exec(ctx, spec)
	if err != nil {
		t.Fatalf("Exec(%q): %v", spec.Command, err)
	}
	return res
}

func TestIntegrationEnsureCreatesRunningSandbox(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)

	insp, err := env.api.ContainerInspect(context.Background(), ContainerName(env.ref))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !insp.State.Running {
		t.Fatal("container is not running after Ensure")
	}
	if insp.Config.Labels[LabelManaged] != "true" {
		t.Error("managed label is missing, so a restarted backend could not find this container")
	}

	res := env.exec(t, sb, ports.ExecSpec{Command: "echo hello from the sandbox"})
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", res.ExitCode, res.Stderr)
	}
	if got := strings.TrimSpace(res.Stdout); got != "hello from the sandbox" {
		t.Errorf("stdout = %q, want %q", got, "hello from the sandbox")
	}

	// The agent runs as the image's non-root user.
	who := env.exec(t, sb, ports.ExecSpec{Command: "id -un; id -u"})
	if strings.Contains(who.Stdout, "root") || strings.Contains(who.Stdout, "\n0\n") {
		t.Errorf("sandbox runs as root: %q", who.Stdout)
	}

	// Egress is closed until the S3 allowlist proxy exists.
	net := env.exec(t, sb, ports.ExecSpec{Command: "curl -sS -m 5 https://example.com", Timeout: 20 * time.Second})
	if net.ExitCode == 0 {
		t.Errorf("outbound HTTP succeeded, want it refused: %q", net.Stdout)
	}
}

func TestIntegrationEnsureIsIdempotent(t *testing.T) {
	env := newIntegration(t, nil)

	first := env.ensure(t)
	second := env.ensure(t)

	if a, b := first.(*sandboxHandle).containerID, second.(*sandboxHandle).containerID; a != b {
		t.Errorf("second Ensure returned a different container: %s vs %s", a, b)
	}
	if got := env.rt.slots.inUse(); got != 1 {
		t.Errorf("slots in use = %d, want 1: a reused sandbox must not consume a second slot", got)
	}

	// Files written before survive the reuse.
	if err := first.WriteFile(context.Background(), "keep.txt", []byte("kept"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := second.ReadFile(context.Background(), "keep.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "kept" {
		t.Errorf("contents = %q, want kept", got)
	}
}

func TestIntegrationControlDirectoryIsMasked(t *testing.T) {
	env := newIntegration(t, nil)

	// Plant a control file host-side, exactly where the real one would sit.
	projectDir := env.rt.ProjectDir(env.ref)
	if err := os.MkdirAll(filepath.Join(projectDir, sandbox.ControlDirName, "threads"), 0o700); err != nil {
		t.Fatalf("seed control dir: %v", err)
	}
	secret := filepath.Join(projectDir, sandbox.ControlDirName, "threads", "secret.json")
	if err := os.WriteFile(secret, []byte(`{"secret":"do not leak"}`), 0o600); err != nil {
		t.Fatalf("seed control file: %v", err)
	}

	sb := env.ensure(t)

	listing := env.exec(t, sb, ports.ExecSpec{Command: "ls -A /workspace/.protean"})
	if strings.TrimSpace(listing.Stdout) != "" {
		t.Errorf("the control directory is visible inside the sandbox: %q", listing.Stdout)
	}

	read := env.exec(t, sb, ports.ExecSpec{Command: "cat /workspace/.protean/threads/secret.json"})
	if read.ExitCode == 0 || strings.Contains(read.Stdout, "do not leak") {
		t.Errorf("a control file was readable inside the sandbox: exit %d, %q", read.ExitCode, read.Stdout)
	}

	write := env.exec(t, sb, ports.ExecSpec{Command: "echo pwned > /workspace/.protean/x"})
	if write.ExitCode == 0 {
		t.Error("the control directory is writable inside the sandbox")
	}

	// The host-side file is untouched by any of it.
	data, err := os.ReadFile(secret)
	if err != nil || !strings.Contains(string(data), "do not leak") {
		t.Errorf("the host-side control file changed: %v, %q", err, data)
	}

	// The tools cannot even name the path.
	if err := sb.WriteFile(context.Background(), ".protean/x", []byte("x"), 0o644); err == nil {
		t.Error("WriteFile accepted a .protean path")
	}
}

func TestIntegrationAgentsFileIsReadOnly(t *testing.T) {
	env := newIntegration(t, nil)

	projectDir := env.rt.ProjectDir(env.ref)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatalf("create project dir: %v", err)
	}
	agents := filepath.Join(projectDir, sandbox.AgentsFileName)
	if err := os.WriteFile(agents, []byte("# project rules\n"), 0o644); err != nil {
		t.Fatalf("seed AGENTS.md: %v", err)
	}

	sb := env.ensure(t)

	read := env.exec(t, sb, ports.ExecSpec{Command: "cat /workspace/AGENTS.md"})
	if !strings.Contains(read.Stdout, "project rules") {
		t.Errorf("AGENTS.md is not readable inside the sandbox: %q", read.Stdout)
	}

	for _, cmd := range []string{
		"echo pwned >> /workspace/AGENTS.md",
		"rm -f /workspace/AGENTS.md",
		"mv /workspace/AGENTS.md /workspace/moved.md",
	} {
		res := env.exec(t, sb, ports.ExecSpec{Command: cmd})
		if res.ExitCode == 0 {
			t.Errorf("%q succeeded, want AGENTS.md to be immutable from inside", cmd)
		}
	}

	data, err := os.ReadFile(agents)
	if err != nil {
		t.Fatalf("read AGENTS.md host-side: %v", err)
	}
	if string(data) != "# project rules\n" {
		t.Errorf("AGENTS.md changed host-side: %q", data)
	}
}

func TestIntegrationAgentsSymlinkCannotEscapeProject(t *testing.T) {
	tests := []struct {
		name   string
		target func(*testing.T, *integrationEnvT) string
	}{
		{"etc passwd", func(*testing.T, *integrationEnvT) string { return "/etc/passwd" }},
		{"other project", func(t *testing.T, env *integrationEnvT) string {
			other := ports.ProjectRef{OrgID: env.ref.OrgID, ProjectID: env.ref.ProjectID + "OTHER"}
			path := filepath.Join(env.rt.ProjectDir(other), sandbox.AgentsFileName)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("other project secret"), 0o644); err != nil {
				t.Fatal(err)
			}
			return path
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newIntegration(t, nil)
			project := env.rt.ProjectDir(env.ref)
			if err := os.MkdirAll(project, 0o755); err != nil {
				t.Fatal(err)
			}
			target := tc.target(t, env)
			agents := filepath.Join(project, sandbox.AgentsFileName)
			if err := os.Symlink(target, agents); err != nil {
				t.Fatal(err)
			}

			sb := env.ensure(t)
			res := env.exec(t, sb, ports.ExecSpec{Command: "cat /workspace/AGENTS.md; test ! -L /workspace/AGENTS.md"})
			if res.ExitCode != 0 || strings.Contains(res.Stdout, "root:") || strings.Contains(res.Stdout, "other project secret") {
				t.Fatalf("unsafe AGENTS.md source reached the sandbox: exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
			}
			info, err := os.Lstat(agents)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				t.Fatal("host AGENTS.md symlink was not replaced")
			}
		})
	}
}

func TestIntegrationControlMaskCoversCaseInsensitiveLookup(t *testing.T) {
	env := newIntegration(t, nil)
	project := env.rt.ProjectDir(env.ref)
	control := filepath.Join(project, sandbox.ControlDirName)
	if err := os.MkdirAll(control, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(control, "case-secret"), []byte("case-insensitive secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	sb := env.ensure(t)

	read := env.exec(t, sb, ports.ExecSpec{Command: "cat /workspace/.PROTEAN/case-secret"})
	if read.ExitCode == 0 || strings.Contains(read.Stdout, "case-insensitive secret") {
		t.Fatalf("alternate-case control path bypassed the mask: exit=%d stdout=%q", read.ExitCode, read.Stdout)
	}
	write := env.exec(t, sb, ports.ExecSpec{Command: "echo escaped > /workspace/.PROTEAN/case-write"})
	if write.ExitCode == 0 {
		t.Fatal("alternate-case control path was writable")
	}
}

func TestIntegrationWriteFileResistsSymlinkSwapRace(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)
	ctx := context.Background()
	project := env.rt.ProjectDir(env.ref)
	agents := filepath.Join(project, sandbox.AgentsFileName)
	if err := os.WriteFile(agents, []byte("agents sentinel"), 0o644); err != nil {
		t.Fatal(err)
	}
	rootTarget := "/tmp/protean-write-race-" + strings.ToLower(env.ref.ProjectID)
	loopDone := make(chan ports.ExecResult, 1)
	loopErr := make(chan error, 1)
	go func() {
		res, err := sb.Exec(ctx, ports.ExecSpec{Command: fmt.Sprintf(`
i=0
while [ "$i" -lt 3000 ]; do
  rm -rf /workspace/parent 2>/dev/null
  mkdir /workspace/parent 2>/dev/null || true
  rm -rf /workspace/parent 2>/dev/null
  ln -s /workspace/.protean /workspace/parent 2>/dev/null || true
  rm -f /workspace/parent 2>/dev/null
  ln -s /workspace/AGENTS.md /workspace/parent 2>/dev/null || true
  rm -f /workspace/parent 2>/dev/null
  ln -s %s /workspace/parent 2>/dev/null || true
  i=$((i+1))
done
`, shellQuote(rootTarget)), Timeout: 30 * time.Second})
		loopDone <- res
		loopErr <- err
	}()

	successes := 0
	for i := range 20 {
		err := sb.WriteFile(ctx, "parent/file", []byte(fmt.Sprintf("write-%d", i)), 0o644)
		if err == nil {
			successes++
		}
	}
	res := <-loopDone
	if err := <-loopErr; err != nil {
		t.Fatalf("symlink swap loop: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("symlink swap loop exit=%d stderr=%q", res.ExitCode, res.Stderr)
	}
	t.Logf("secure writes completed during race: %d/20", successes)

	if _, err := os.Stat(filepath.Join(controlPath(project), "file")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("write escaped into .protean: %v", err)
	}
	if got, err := os.ReadFile(agents); err != nil || string(got) != "agents sentinel" {
		t.Fatalf("AGENTS.md changed during race: %v, %q", err, got)
	}
	root := env.exec(t, sb, ports.ExecSpec{Command: fmt.Sprintf("test ! -e %s/file", shellQuote(rootTarget))})
	if root.ExitCode != 0 {
		t.Fatal("write escaped into a rootfs location")
	}
}

func controlPath(project string) string { return filepath.Join(project, sandbox.ControlDirName) }

func TestIntegrationHostilePathCannotReplaceTimeoutSupervisor(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)
	if err := sb.WriteFile(context.Background(), "timeout", []byte("#!/bin/sh\necho hijacked > /workspace/timeout-hijacked\nexec \"$@\"\n"), 0o755); err != nil {
		t.Fatalf("write hostile timeout: %v", err)
	}
	res := env.exec(t, sb, ports.ExecSpec{Command: "sleep 20", Env: []string{"PATH=/workspace"}, Timeout: time.Second})
	if !res.TimedOut {
		t.Fatalf("real timeout did not fire: exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
	}
	marker := env.exec(t, sb, ports.ExecSpec{Command: "test ! -e /workspace/timeout-hijacked"})
	if marker.ExitCode != 0 {
		t.Fatal("agent-written timeout was executed")
	}
}

func TestIntegrationWorkspaceIsWritableBothWays(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)
	ctx := context.Background()

	if err := sb.WriteFile(ctx, "src/deep/hello.txt", []byte("from the host"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// The sandbox user owns what the tools write, or it could not edit them.
	owner := env.exec(t, sb, ports.ExecSpec{Command: "stat -c '%U %a' /workspace/src/deep/hello.txt"})
	if !strings.HasPrefix(strings.TrimSpace(owner.Stdout), "agent ") {
		t.Errorf("written file owner/mode = %q, want it owned by agent", strings.TrimSpace(owner.Stdout))
	}
	appended := env.exec(t, sb, ports.ExecSpec{Command: "echo ' and the sandbox' >> src/deep/hello.txt", Cwd: ""})
	if appended.ExitCode != 0 {
		t.Fatalf("the sandbox cannot write its own file: %q", appended.Stderr)
	}

	got, err := sb.ReadFile(ctx, "src/deep/hello.txt")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(got), "from the host") || !strings.Contains(string(got), "and the sandbox") {
		t.Errorf("contents = %q, want both writes", got)
	}

	// The host sees the same bytes through the bind mount.
	hostPath := filepath.Join(env.rt.ProjectDir(env.ref), "src", "deep", "hello.txt")
	hostData, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("read through the bind mount: %v", err)
	}
	if string(hostData) != string(got) {
		t.Errorf("host sees %q, sandbox sees %q", hostData, got)
	}

	if err := sb.DeleteFile(ctx, "src/deep/hello.txt"); err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if _, err := sb.ReadFile(ctx, "src/deep/hello.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ReadFile after delete err = %v, want fs.ErrNotExist", err)
	}
	if err := sb.DeleteFile(ctx, "src/deep/hello.txt"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("second DeleteFile err = %v, want fs.ErrNotExist", err)
	}
}

func TestIntegrationExecTimesOut(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)

	started := time.Now()
	res := env.exec(t, sb, ports.ExecSpec{Command: "sleep 60", Timeout: 2 * time.Second})
	elapsed := time.Since(started)

	if !res.TimedOut {
		t.Errorf("TimedOut = false, want true (exit %d)", res.ExitCode)
	}
	if res.ExitCode == 0 {
		t.Error("exit code = 0, want a kill-shaped code")
	}
	if elapsed > 30*time.Second {
		t.Errorf("the command ran for %s, want it killed at about 2s", elapsed)
	}

	background := env.exec(t, sb, ports.ExecSpec{Command: "nohup sleep 3600 >/dev/null 2>&1 & echo $!"})
	pid := strings.TrimSpace(background.Stdout)
	if pid == "" {
		t.Fatal("background-process probe did not return a pid")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		survived := env.exec(t, sb, ports.ExecSpec{Command: "! pgrep -f '^sleep 3600$' >/dev/null"})
		if survived.ExitCode == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background process %s survived its leader exit", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// The whole process tree dies, not just the shell.
	tree := env.exec(t, sb, ports.ExecSpec{Command: "sleep 60 & sleep 60", Timeout: 2 * time.Second})
	if !tree.TimedOut {
		t.Errorf("TimedOut = false for a command with a background child (exit %d)", tree.ExitCode)
	}
	survivors := env.exec(t, sb, ports.ExecSpec{Command: "ls /proc | grep -c '^[0-9]*$'"})
	t.Logf("processes left in the sandbox: %s", strings.TrimSpace(survivors.Stdout))

	// The sandbox is still usable afterwards.
	after := env.exec(t, sb, ports.ExecSpec{Command: "echo still alive"})
	if strings.TrimSpace(after.Stdout) != "still alive" {
		t.Errorf("the sandbox is broken after a timeout: %q / %q", after.Stdout, after.Stderr)
	}
}

func TestIntegrationExecCapsOutput(t *testing.T) {
	env := newIntegration(t, func(o *Options) { o.OutputCapBytes = 2048 })
	sb := env.ensure(t)
	before := sb.(*sandboxHandle).containerID

	res := env.exec(t, sb, ports.ExecSpec{
		Command: "head -c 200000 /dev/zero | tr '\\0' 'a'",
		Timeout: 20 * time.Second,
	})
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(res.Stdout) > 2048 {
		t.Errorf("stdout kept %d bytes, want at most 2048", len(res.Stdout))
	}

	// A runaway writer is bounded too, and the sandbox survives it.
	flood := env.exec(t, sb, ports.ExecSpec{Command: "yes aaaaaaaaaaaaaaaa", Timeout: 5 * time.Second})
	if !flood.Truncated {
		t.Error("an infinite writer was not reported as truncated")
	}
	if len(flood.Stdout) > 2048 {
		t.Errorf("stdout kept %d bytes from the flood, want at most 2048", len(flood.Stdout))
	}
	if afterID := sb.(*sandboxHandle).containerID; afterID == before {
		t.Errorf("output cutoff did not recreate the sandbox: container id remained %s", afterID)
	}
	after := env.exec(t, sb, ports.ExecSpec{Command: "echo ok"})
	if strings.TrimSpace(after.Stdout) != "ok" {
		t.Errorf("the sandbox is broken after a flood: %q / %q", after.Stdout, after.Stderr)
	}
}

func TestIntegrationExecCancellationRecreatesSandbox(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)
	before := sb.(*sandboxHandle).containerID
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_, err := sb.Exec(ctx, ports.ExecSpec{Command: "sleep 60", Timeout: 30 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "sandbox was recreated") {
		t.Fatalf("cancelled Exec error = %v, want a sandbox-recreated error", err)
	}
	after := sb.(*sandboxHandle).containerID
	if after == before {
		t.Fatalf("container id remained %s after cancellation", after)
	}
	insp, err := env.api.ContainerInspect(context.Background(), after)
	if err != nil || insp.State == nil || !insp.State.Running {
		t.Fatalf("replacement sandbox is not running: %v, %+v", err, insp.State)
	}
	check := env.exec(t, sb, ports.ExecSpec{Command: "! pgrep -f '^sleep 60$' >/dev/null"})
	if check.ExitCode != 0 {
		t.Fatal("cancelled exec process survived sandbox recreation")
	}
}

func TestIntegrationResourceLimitsApply(t *testing.T) {
	env := newIntegration(t, nil)
	sb := env.ensure(t)

	insp, err := env.api.ContainerInspect(context.Background(), ContainerName(env.ref))
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if insp.HostConfig.NanoCPUs != DefaultNanoCPUs {
		t.Errorf("NanoCPUs = %d, want %d", insp.HostConfig.NanoCPUs, int64(DefaultNanoCPUs))
	}
	if insp.HostConfig.Memory != DefaultMemoryBytes {
		t.Errorf("Memory = %d, want %d", insp.HostConfig.Memory, int64(DefaultMemoryBytes))
	}
	if insp.HostConfig.MemorySwap != DefaultMemoryBytes {
		t.Errorf("MemorySwap = %d, want %d", insp.HostConfig.MemorySwap, int64(DefaultMemoryBytes))
	}
	if insp.HostConfig.PidsLimit == nil || *insp.HostConfig.PidsLimit != DefaultPidsLimit {
		t.Errorf("PidsLimit = %v, want %d", insp.HostConfig.PidsLimit, DefaultPidsLimit)
	}

	// The pids cap is observable: a fork loop hits it instead of the host.
	fork := env.exec(t, sb, ports.ExecSpec{
		Command: "i=0; while [ $i -lt 400 ]; do sleep 30 & i=$((i+1)); done; echo spawned=$i",
		Timeout: 30 * time.Second,
	})
	combined := fork.Stdout + fork.Stderr
	if fork.ExitCode == 0 && !strings.Contains(combined, "annot") && !strings.Contains(combined, "esource") {
		t.Errorf("a 400-process fork loop succeeded, want the pids limit to stop it: exit %d, %q",
			fork.ExitCode, combined)
	} else {
		t.Logf("fork loop stopped as expected: exit %d, %q", fork.ExitCode, strings.TrimSpace(combined))
	}
}

func TestIntegrationStopStartDestroy(t *testing.T) {
	env := newIntegration(t, nil)
	ctx := context.Background()

	sb := env.ensure(t)
	if err := sb.WriteFile(ctx, "persisted.txt", []byte("survives"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := env.rt.Stop(ctx, env.ref); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	insp, err := env.api.ContainerInspect(ctx, ContainerName(env.ref))
	if err != nil {
		t.Fatalf("inspect after Stop: %v", err)
	}
	if insp.State.Running {
		t.Error("the container is still running after Stop")
	}
	if got := env.rt.slots.inUse(); got != 0 {
		t.Errorf("slots in use after Stop = %d, want 0", got)
	}

	// A handle taken before the stop restarts the sandbox on its next use.
	res := env.exec(t, sb, ports.ExecSpec{Command: "cat persisted.txt"})
	if strings.TrimSpace(res.Stdout) != "survives" {
		t.Errorf("stdout = %q, want the file from before the stop", res.Stdout)
	}

	if err := env.rt.Destroy(ctx, env.ref); err != nil {
		t.Fatalf("Destroy: %v", err)
	}
	if _, err := env.api.ContainerInspect(ctx, ContainerName(env.ref)); !client.IsErrNotFound(err) {
		t.Errorf("inspect after Destroy err = %v, want a 404", err)
	}
	// Destroy keeps the project files.
	hostPath := filepath.Join(env.rt.ProjectDir(env.ref), "persisted.txt")
	if data, err := os.ReadFile(hostPath); err != nil || string(data) != "survives" {
		t.Errorf("project files did not survive Destroy: %v, %q", err, data)
	}
}

func TestIntegrationIdleReaperStopsSandbox(t *testing.T) {
	env := newIntegration(t, func(o *Options) {
		o.IdleTimeout = 500 * time.Millisecond
		o.ReapInterval = 100 * time.Millisecond
	})
	env.ensure(t)

	ctx, cancel := context.WithCancel(context.Background())
	env.rt.Start(ctx)
	defer func() {
		cancel()
		env.rt.Wait()
	}()

	deadline := time.Now().Add(30 * time.Second)
	for {
		insp, err := env.api.ContainerInspect(context.Background(), ContainerName(env.ref))
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if !insp.State.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the idle sandbox was never reaped")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if got := env.rt.slots.inUse(); got != 0 {
		t.Errorf("slots in use after reaping = %d, want 0", got)
	}
}

func TestIntegrationIdleReaperDoesNotStopActiveExec(t *testing.T) {
	env := newIntegration(t, func(o *Options) {
		o.IdleTimeout = 300 * time.Millisecond
		o.ReapInterval = 50 * time.Millisecond
	})
	sb := env.ensure(t)
	ctx, cancel := context.WithCancel(context.Background())
	env.rt.Start(ctx)
	defer func() {
		cancel()
		env.rt.Wait()
	}()
	res := env.exec(t, sb, ports.ExecSpec{Command: "sleep 1; echo completed", Timeout: 5 * time.Second})
	if res.ExitCode != 0 || strings.TrimSpace(res.Stdout) != "completed" {
		t.Fatalf("active exec was interrupted: exit=%d stdout=%q stderr=%q", res.ExitCode, res.Stdout, res.Stderr)
	}
	insp, err := env.api.ContainerInspect(context.Background(), ContainerName(env.ref))
	if err != nil || insp.State == nil || !insp.State.Running {
		t.Fatalf("sandbox was stopped during or immediately after active exec: %v", err)
	}
}

func TestIntegrationAdoptsRunningContainersAfterRestart(t *testing.T) {
	env := newIntegration(t, nil)
	env.ensure(t)

	// A second runtime on the same daemon models a backend restart.
	restarted, err := New(context.Background(), env.api, Options{
		Image:      env.rt.opts.Image,
		DataDir:    env.dir,
		MaxRunning: 2,
		Logger:     slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := restarted.slots.inUse(); got < 1 {
		t.Errorf("slots in use after restart = %d, want the running sandbox counted", got)
	}

	sb, err := restarted.Ensure(context.Background(), env.ref)
	if err != nil {
		t.Fatalf("Ensure after restart: %v", err)
	}
	res, err := sb.Exec(context.Background(), ports.ExecSpec{Command: "echo adopted"})
	if err != nil {
		t.Fatalf("Exec after restart: %v", err)
	}
	if strings.TrimSpace(res.Stdout) != "adopted" {
		t.Errorf("stdout = %q, want adopted", res.Stdout)
	}
}

func TestIntegrationConcurrencyCapQueues(t *testing.T) {
	env := newIntegration(t, func(o *Options) { o.MaxRunning = 1 })
	ctx := context.Background()

	second := ports.ProjectRef{OrgID: env.ref.OrgID, ProjectID: env.ref.ProjectID + "B"}
	t.Cleanup(func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := env.rt.Destroy(cleanupCtx, second); err != nil {
			t.Logf("teardown: destroy %s: %v", ContainerName(second), err)
		}
	})

	env.ensure(t)

	blocked, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	if _, err := env.rt.Ensure(blocked, second); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second Ensure err = %v, want it to queue and then hit the deadline", err)
	}

	if err := env.rt.Stop(ctx, env.ref); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	waitCtx, cancel2 := context.WithTimeout(ctx, 60*time.Second)
	defer cancel2()
	if _, err := env.rt.Ensure(waitCtx, second); err != nil {
		t.Fatalf("second Ensure after the slot freed: %v", err)
	}
}

// leftoverCheck is a belt-and-braces guard: nothing this package's tests create
// may outlive the run.
func TestIntegrationNoLeftoverContainers(t *testing.T) {
	env := newIntegration(t, nil)
	env.ensure(t)
	if err := env.rt.Destroy(context.Background(), env.ref); err != nil {
		t.Fatalf("Destroy: %v", err)
	}

	list, err := env.api.ContainerList(context.Background(), container.ListOptions{All: true})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, c := range list {
		for _, name := range c.Names {
			if strings.Contains(name, "/"+ContainerName(env.ref)) {
				t.Errorf("container %s outlived Destroy", name)
			}
		}
	}
}
