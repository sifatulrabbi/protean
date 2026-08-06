package ports

import (
	"context"
	"io/fs"
	"time"
)

// ProjectRef identifies the project a sandbox belongs to. Both fields are ULID
// strings; a sandbox is always scoped to exactly one project.
type ProjectRef struct {
	OrgID     string
	ProjectID string
}

// SandboxRuntime creates and owns the per-project sandboxes. It is the security
// boundary of the platform: the agent harness runs on the host and only tool
// execution crosses into a sandbox, so nothing here ever receives an API key,
// the database, or another project's files.
//
// Every implementation must uphold the same contract:
//
//   - The project directory is the only host state a sandbox can reach, mounted
//     read-write at the workspace root.
//   - `.protean/` is masked inside the sandbox. The agent cannot read, list, or
//     write the control files (threads, memories, skills) that live there, and
//     no path accepted by Sandbox may address them.
//   - `AGENTS.md` is mounted read-only. Its edits are approval-gated and happen
//     host-side, never from inside.
//   - CPU, memory, and process-count limits are applied by the runtime, not
//     requested by the caller.
//   - Sandboxes are cheap when idle: an idle sandbox is stopped automatically
//     and the project files stay on the host.
//
// There is no unsandboxed implementation. When no runtime is healthy the
// process must fail to boot rather than run tools on the host.
type SandboxRuntime interface {
	// Ensure returns a running sandbox for ref, creating and starting it when
	// needed and reusing it when it already runs. It blocks while the runtime
	// is at its global concurrency cap and returns ctx.Err() if ctx ends first.
	Ensure(ctx context.Context, ref ProjectRef) (Sandbox, error)

	// Stop stops the sandbox and frees its concurrency slot, keeping the
	// project files and the container itself. It is a no-op for a sandbox that
	// does not exist or is already stopped. The idle reaper calls it.
	Stop(ctx context.Context, ref ProjectRef) error

	// Destroy removes the sandbox entirely, for project deletion or an image
	// upgrade. Project files on the host are untouched. It is a no-op for a
	// sandbox that does not exist.
	Destroy(ctx context.Context, ref ProjectRef) error

	// Start launches the runtime's background work — the idle reaper — and
	// returns immediately. The work stops when ctx is cancelled; Wait blocks
	// until it has. This mirrors the entitlements watcher.
	Start(ctx context.Context)

	// Wait blocks until the background work started by Start has stopped.
	Wait()

	// Close releases the runtime's own resources (daemon connections). It does
	// not stop running sandboxes: they are reclaimed on the next boot.
	Close() error
}

// Sandbox is one project's isolated execution environment. It is the backend of
// the Bash, Grep, Glob, and file tools.
//
// Every path argument is relative to the workspace root and is rejected when it
// is absolute, escapes the workspace via "..", or names the `.protean` control
// directory in any segment. Callers therefore cannot address host paths, other
// projects, or control files, whatever the agent asks for.
type Sandbox interface {
	// Ref is the project this sandbox belongs to.
	Ref() ProjectRef

	// Exec runs one command inside the sandbox and returns when it finishes,
	// its timeout expires, or ctx ends. A non-zero exit code is reported in the
	// result, not as an error: only failures to run the command at all are
	// errors. The runtime — not the caller — enforces the timeout ceiling and
	// the output caps, so a runaway command cannot hang or flood the harness.
	Exec(ctx context.Context, spec ExecSpec) (ExecResult, error)

	// WriteFile writes data at the workspace-relative path, creating parent
	// directories as needed and leaving the file owned by the sandbox user.
	// Writing to AGENTS.md fails: it is mounted read-only by contract.
	WriteFile(ctx context.Context, path string, data []byte, mode fs.FileMode) error

	// ReadFile returns the contents of the workspace-relative path. A missing
	// file fails with an error matching fs.ErrNotExist.
	ReadFile(ctx context.Context, path string) ([]byte, error)

	// DeleteFile removes the workspace-relative path, recursively when it is a
	// directory. A missing path fails with an error matching fs.ErrNotExist.
	DeleteFile(ctx context.Context, path string) error
}

// ExecSpec is one command to run inside a sandbox.
type ExecSpec struct {
	// Command is a shell command line, run by /bin/sh.
	Command string

	// Cwd is the working directory relative to the workspace root. Empty means
	// the workspace root itself.
	Cwd string

	// Env are extra "KEY=value" entries layered on the image environment.
	// Secrets must never appear here: they stay on the host (D11).
	Env []string

	// Timeout bounds this command. Zero uses the runtime default; a value above
	// the runtime's hard ceiling is clamped down to it.
	Timeout time.Duration
}

// ExecResult is the outcome of one ExecSpec.
type ExecResult struct {
	// ExitCode is the command's exit status, or 128+signal when it was killed.
	ExitCode int

	// Stdout and Stderr hold the captured output, each cut at the runtime's
	// per-stream cap.
	Stdout string
	Stderr string

	// Truncated reports that at least one stream produced more output than the
	// cap and what is in this result is a prefix.
	Truncated bool

	// TimedOut reports that the command was killed because it outlived its
	// timeout rather than exiting on its own.
	TimedOut bool
}
