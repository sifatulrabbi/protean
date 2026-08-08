package dockerbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/sandbox"
)

const (
	// killGrace is how long the in-container `timeout` waits after SIGTERM
	// before it sends SIGKILL.
	killGrace = 5 * time.Second

	// attachSlack is the extra time the host waits for the stream to close
	// after the in-container timeout should have fired. It is the safety net
	// for a wedged daemon connection, not the primary mechanism.
	attachSlack = 10 * time.Second

	// exitTimedOut and exitKilled are the exit codes `timeout` reports when it
	// terminated the command with SIGTERM and with SIGKILL respectively.
	exitTimedOut = 124
	exitKilled   = 137

	// timeoutSlop absorbs measurement jitter when deciding whether a kill-shaped
	// exit code really was our timeout firing.
	timeoutSlop = 500 * time.Millisecond

	// inspectPoll bounds the wait for the daemon to publish the exit code after
	// the output stream closes.
	inspectPollInterval = 20 * time.Millisecond
	inspectPollTimeout  = 3 * time.Second
)

var errExecStillRunning = errors.New("sandbox exec is still running after its termination deadline")

// sandboxHandle implements ports.Sandbox. It re-resolves the container before
// every operation so a handle stays usable after the idle reaper stopped the
// sandbox underneath it.
type sandboxHandle struct {
	rt          *Runtime
	ref         ports.ProjectRef
	containerID string
}

var _ ports.Sandbox = (*sandboxHandle)(nil)

func (s *sandboxHandle) Ref() ports.ProjectRef { return s.ref }

// container returns a running container id, restarting the sandbox if needed.
func (s *sandboxHandle) container(ctx context.Context) (string, error) {
	id, err := s.rt.ensure(ctx, s.ref)
	if err != nil {
		return "", err
	}
	s.containerID = id
	s.rt.touch(s.ref)
	return id, nil
}

// Exec runs one shell command inside the sandbox.
//
// The timeout is enforced inside the container by coreutils `timeout`, which
// puts the command in its own process group and signals the whole group — so a
// command that spawned children takes its process tree down with it. The host
// deadline below it only guards against a stuck daemon connection.
func (s *sandboxHandle) Exec(ctx context.Context, spec ports.ExecSpec) (ports.ExecResult, error) {
	if strings.TrimSpace(spec.Command) == "" {
		return ports.ExecResult{}, errors.New("sandbox: exec command is empty")
	}

	workDir, err := sandbox.ResolveDir(spec.Cwd)
	if err != nil {
		return ports.ExecResult{}, fmt.Errorf("sandbox: exec cwd: %w", err)
	}
	id, finish, err := s.rt.beginOperation(ctx, s.ref)
	if err != nil {
		return ports.ExecResult{}, err
	}
	defer finish()
	s.containerID = id

	timeout := s.rt.execTimeout(spec.Timeout)
	execCfg := container.ExecOptions{
		Cmd:          wrapCommand(spec.Command, timeout),
		WorkingDir:   workDir,
		Env:          safeExecEnv(spec.Env),
		AttachStdout: true,
		AttachStderr: true,
	}

	created, err := s.rt.api.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return ports.ExecResult{}, fmt.Errorf("create exec in %s: %w", ContainerName(s.ref), err)
	}

	// The host deadline sits deliberately past the in-container one.
	attachCtx, cancel := context.WithTimeout(ctx, timeout+killGrace+attachSlack)
	defer cancel()

	resp, err := s.rt.api.ContainerExecAttach(attachCtx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return ports.ExecResult{}, fmt.Errorf("attach exec in %s: %w", ContainerName(s.ref), err)
	}
	defer resp.Close()

	started := s.rt.clock.Now()
	capBytes := s.rt.opts.OutputCapBytes
	stdout := &capWriter{limit: capBytes}
	stderr := &capWriter{limit: capBytes}

	// Closing the hijacked connection is what unblocks a copy that the host
	// deadline has outlived; it also makes a flooding command die on SIGPIPE.
	closed := make(chan struct{})
	copyDone := make(chan struct{})
	go func() {
		select {
		case <-attachCtx.Done():
			close(closed)
			resp.Close()
		case <-copyDone:
		}
	}()

	drain := &drainReader{r: resp.Reader, budget: drainBudget(capBytes), onOver: resp.Close}
	_, copyErr := stdcopy.StdCopy(stdout, stderr, drain)
	close(copyDone)

	hostDeadlineFired := false
	select {
	case <-closed:
		hostDeadlineFired = true
	default:
	}

	if copyErr != nil && !drain.over && !hostDeadlineFired {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ports.ExecResult{}, s.resetExecError(ctxErr)
		}
		return ports.ExecResult{}, fmt.Errorf("read exec output from %s: %w", ContainerName(s.ref), copyErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ports.ExecResult{}, s.resetExecError(ctxErr)
	}
	if drain.over || hostDeadlineFired {
		reason := "output cutoff"
		if hostDeadlineFired {
			reason = "host execution deadline"
		}
		if err := s.resetAfterExec(reason); err != nil {
			return ports.ExecResult{}, err
		}
		return ports.ExecResult{
			ExitCode:  exitKilled,
			Stdout:    stdout.String(),
			Stderr:    stderr.String() + "\nprotean: sandbox recreated after " + reason + "\n",
			Truncated: true,
			TimedOut:  hostDeadlineFired,
		}, nil
	}

	exitCode, err := s.waitExit(ctx, created.ID)
	if err != nil {
		if errors.Is(err, errExecStillRunning) {
			return ports.ExecResult{}, s.resetExecError(err)
		}
		return ports.ExecResult{}, err
	}

	elapsed := s.rt.clock.Now().Sub(started)
	killShaped := exitCode == exitTimedOut || exitCode == exitKilled
	result := ports.ExecResult{
		ExitCode:  exitCode,
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		Truncated: stdout.truncated() || stderr.truncated() || drain.over,
		TimedOut:  hostDeadlineFired || (killShaped && elapsed+timeoutSlop >= timeout),
	}
	return result, nil
}

func safeExecEnv(env []string) []string {
	critical := map[string]bool{
		"PATH": true, "IFS": true, "ENV": true, "BASH_ENV": true, "SHELLOPTS": true,
		"LD_PRELOAD": true, "LD_LIBRARY_PATH": true,
	}
	out := make([]string, 0, len(env)+1)
	for _, entry := range env {
		name, _, ok := strings.Cut(entry, "=")
		if !ok || critical[name] {
			continue
		}
		out = append(out, entry)
	}
	return append(out, "PATH=/usr/local/bin:/usr/bin:/bin")
}

func (s *sandboxHandle) resetExecError(cause error) error {
	if err := s.resetAfterExec(cause.Error()); err != nil {
		return errors.Join(cause, err)
	}
	return fmt.Errorf("%w; sandbox was recreated to guarantee process termination", cause)
}

func (s *sandboxHandle) resetAfterExec(reason string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := s.rt.replaceContainer(ctx, s.ref); err != nil {
		return fmt.Errorf("sandbox: could not recreate %s after %s: %w", ContainerName(s.ref), reason, err)
	}
	insp, err := s.rt.api.ContainerInspect(ctx, ContainerName(s.ref))
	if err != nil {
		return fmt.Errorf("sandbox: inspect recreated %s after %s: %w", ContainerName(s.ref), reason, err)
	}
	s.containerID = insp.ID
	return nil
}

// execInput is the narrow stdin-capable exec used by WriteFile. Supplying file
// bytes over the hijacked exec stream avoids Docker tar extraction, whose path
// walk can be raced through workspace symlinks.
func (s *sandboxHandle) execInput(ctx context.Context, id, command string, input []byte, timeout time.Duration) (ports.ExecResult, error) {
	execCfg := container.ExecOptions{
		Cmd:          wrapInputCommand(command, timeout),
		WorkingDir:   sandbox.WorkspaceRoot,
		Env:          safeExecEnv(nil),
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
	}
	created, err := s.rt.api.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return ports.ExecResult{}, err
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout+killGrace+attachSlack)
	defer cancel()
	resp, err := s.rt.api.ContainerExecAttach(execCtx, created.ID, container.ExecAttachOptions{})
	if err != nil {
		return ports.ExecResult{}, err
	}
	defer resp.Close()

	writeDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(resp.Conn, bytes.NewReader(input))
		if closeErr := resp.CloseWrite(); err == nil {
			err = closeErr
		}
		writeDone <- err
	}()
	stdout := &capWriter{limit: s.rt.opts.OutputCapBytes}
	stderr := &capWriter{limit: s.rt.opts.OutputCapBytes}
	_, copyErr := stdcopy.StdCopy(stdout, stderr, resp.Reader)
	writeErr := <-writeDone
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ports.ExecResult{}, s.resetExecError(ctxErr)
	}
	if execCtx.Err() != nil {
		return ports.ExecResult{}, s.resetExecError(execCtx.Err())
	}
	if writeErr != nil && !errors.Is(writeErr, io.ErrClosedPipe) {
		return ports.ExecResult{}, fmt.Errorf("send exec input: %w", writeErr)
	}
	if copyErr != nil {
		return ports.ExecResult{}, fmt.Errorf("read exec output: %w", copyErr)
	}
	exitCode, err := s.waitExit(ctx, created.ID)
	if err != nil {
		if errors.Is(err, errExecStillRunning) {
			return ports.ExecResult{}, s.resetExecError(err)
		}
		return ports.ExecResult{}, err
	}
	return ports.ExecResult{ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String()}, nil
}

func wrapInputCommand(command string, timeout time.Duration) []string {
	secs := int(timeout.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return []string{
		"/usr/bin/timeout",
		"--kill-after=" + strconv.Itoa(int(killGrace/time.Second)) + "s",
		strconv.Itoa(secs) + "s",
		"/usr/bin/setsid", "/bin/sh", "-lc", command,
	}
}

// waitExit polls until the daemon reports the exec finished, then returns its
// exit code.
func (s *sandboxHandle) waitExit(ctx context.Context, execID string) (int, error) {
	deadline := time.Now().Add(inspectPollTimeout)
	for {
		insp, err := s.rt.api.ContainerExecInspect(ctx, execID)
		if err != nil {
			return 0, fmt.Errorf("inspect exec in %s: %w", ContainerName(s.ref), err)
		}
		if !insp.Running {
			return insp.ExitCode, nil
		}
		if time.Now().After(deadline) {
			return 0, errExecStillRunning
		}
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(inspectPollInterval):
		}
	}
}

// execTimeout resolves the effective timeout: the caller's value, the
// configured default when unset, never above the hard ceiling.
func (r *Runtime) execTimeout(requested time.Duration) time.Duration {
	if requested <= 0 {
		requested = r.opts.ExecTimeout
	}
	if requested > MaxExecTimeout {
		requested = MaxExecTimeout
	}
	if requested < time.Second {
		requested = time.Second
	}
	return requested
}

// wrapCommand builds the argv the daemon runs. The absolute timeout path cannot
// be replaced by a workspace file or hostile PATH. The inner shell starts in a
// new session, and the outer supervisor always kills that whole process group,
// including background work, before it exits.
func wrapCommand(command string, timeout time.Duration) []string {
	secs := int(timeout.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return []string{
		"/usr/bin/timeout",
		"--kill-after=" + strconv.Itoa(int(killGrace/time.Second)) + "s",
		strconv.Itoa(secs) + "s",
		"/bin/sh", "-c", `
child=
cleanup() {
  rc=$?
  trap - EXIT HUP INT TERM
  if [ -n "$child" ]; then
    /bin/kill -TERM -- "-$child" 2>/dev/null || true
    i=0
    while [ "$i" -lt 10 ] && /bin/kill -0 -- "-$child" 2>/dev/null; do
      /bin/sleep 0.1
      i=$((i+1))
    done
    /bin/kill -KILL -- "-$child" 2>/dev/null || true
    wait "$child" 2>/dev/null || true
  fi
  exit "$rc"
}
trap cleanup EXIT HUP INT TERM
/usr/bin/setsid /bin/sh -lc "$1" <&0 &
child=$!
wait "$child"
exit $?
`, "protean-supervisor", command,
	}
}

// capWriter keeps the first limit bytes and counts the rest, so the caller
// learns that output was cut without the host buffering a runaway stream.
type capWriter struct {
	limit int
	buf   []byte
	seen  int64
}

func (w *capWriter) Write(p []byte) (int, error) {
	w.seen += int64(len(p))
	if room := w.limit - len(w.buf); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		w.buf = append(w.buf, p[:room]...)
	}
	return len(p), nil
}

func (w *capWriter) String() string  { return string(w.buf) }
func (w *capWriter) truncated() bool { return w.seen > int64(w.limit) }

// drainBudget is how much output the host will read past the caps before it
// cuts the connection. Draining past the cap keeps the command from blocking on
// a full pipe; the budget keeps an infinite writer from spinning until its
// timeout.
func drainBudget(capBytes int) int64 {
	budget := int64(capBytes) * 8
	if budget < 1<<20 {
		budget = 1 << 20
	}
	return budget
}

// drainReader stops the stream once the drain budget is spent and reports it.
type drainReader struct {
	r      io.Reader
	budget int64
	onOver func()
	over   bool
}

func (d *drainReader) Read(p []byte) (int, error) {
	if d.over {
		return 0, io.EOF
	}
	n, err := d.r.Read(p)
	d.budget -= int64(n)
	if d.budget <= 0 {
		d.over = true
		if d.onOver != nil {
			d.onOver()
		}
		if err == nil {
			err = io.EOF
		}
	}
	return n, err
}
