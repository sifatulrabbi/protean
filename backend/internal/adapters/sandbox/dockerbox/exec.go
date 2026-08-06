package dockerbox

import (
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
	id, err := s.container(ctx)
	if err != nil {
		return ports.ExecResult{}, err
	}
	if strings.TrimSpace(spec.Command) == "" {
		return ports.ExecResult{}, errors.New("sandbox: exec command is empty")
	}

	workDir, err := sandbox.ResolveDir(spec.Cwd)
	if err != nil {
		return ports.ExecResult{}, fmt.Errorf("sandbox: exec cwd: %w", err)
	}

	timeout := s.rt.execTimeout(spec.Timeout)
	execCfg := container.ExecOptions{
		Cmd:          wrapCommand(spec.Command, timeout),
		WorkingDir:   workDir,
		Env:          spec.Env,
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
			return ports.ExecResult{}, ctxErr
		}
		return ports.ExecResult{}, fmt.Errorf("read exec output from %s: %w", ContainerName(s.ref), copyErr)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ports.ExecResult{}, ctxErr
	}

	exitCode, err := s.waitExit(ctx, created.ID)
	if err != nil {
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
			return insp.ExitCode, nil
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

// wrapCommand builds the argv the daemon runs. `timeout` is the process-group
// killer; `sh -lc` is the shell the agent's command line expects.
func wrapCommand(command string, timeout time.Duration) []string {
	secs := int(timeout.Round(time.Second) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return []string{
		"timeout",
		"--kill-after=" + strconv.Itoa(int(killGrace/time.Second)) + "s",
		strconv.Itoa(secs) + "s",
		"/bin/sh", "-lc", command,
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
