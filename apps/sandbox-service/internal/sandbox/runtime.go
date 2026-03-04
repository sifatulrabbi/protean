package sandbox

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

var ErrContainerNotFound = errors.New("sandbox container not found")

// ContainerState describes the current Docker container backing a session.
type ContainerState struct {
	// ID is the Docker container ID.
	ID string
	// Name is the stable Docker container name used by the service.
	Name string
	// Image is the image currently configured on the container.
	Image string
	// State is Docker's human-readable container status.
	State string
	// Present reports whether Docker currently knows about the container.
	Present bool
	// Running reports whether the container is actively running.
	Running bool
}

// ExecResult captures command execution output returned to API callers.
type ExecResult struct {
	// ExitCode is the process exit code, or -1 when the command timed out before an exit code was available.
	ExitCode int `json:"exitCode"`
	// Stdout contains captured standard output, truncated when it exceeds the configured limit.
	Stdout string `json:"stdout"`
	// Stderr contains captured standard error, truncated when it exceeds the configured limit.
	Stderr string `json:"stderr"`
	// TimedOut reports whether the command exceeded the requested timeout.
	TimedOut bool `json:"timedOut"`
	// SignalCode contains the decoded signal name when the exit code matches the 128+signal convention.
	SignalCode string `json:"signalCode"`
}

// Runtime abstracts container lifecycle and command execution for sandbox sessions.
type Runtime interface {
	InspectContainer(ctx context.Context, meta SessionMetadata) (ContainerState, error)
	EnsureContainer(ctx context.Context, meta SessionMetadata) (ContainerState, error)
	RemoveContainer(ctx context.Context, meta SessionMetadata) error
	Exec(
		ctx context.Context,
		meta SessionMetadata,
		containerCwd string,
		command string,
		timeout time.Duration,
		maxOutputBytes int,
	) (ExecResult, error)
}

// DockerClient is a narrow interface matching only the Docker SDK methods we call.
// This enables test mocks without heavy SDK test fixtures.
type DockerClient interface {
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
	ContainerCreate(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *ocispec.Platform, containerName string) (container.CreateResponse, error)
	ContainerStart(ctx context.Context, containerID string, options container.StartOptions) error
	ContainerRemove(ctx context.Context, containerID string, options container.RemoveOptions) error
	ContainerExecCreate(ctx context.Context, containerID string, config container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
}

// DockerRuntime implements Runtime using the Docker Engine API.
type DockerRuntime struct {
	// containerUser is passed to Docker so commands run with the expected UID/GID.
	containerUser string
	// workspaceMountPath is the in-container bind mount target for session workspaces.
	workspaceMountPath string
	// docker is the Docker client implementation used for SDK calls.
	docker DockerClient
}

// NewDockerRuntime creates a DockerRuntime with a real Docker SDK client.
func NewDockerRuntime(containerUser, workspaceMountPath string) (*DockerRuntime, error) {
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("create docker client: %w", err)
	}
	return &DockerRuntime{
		containerUser:      containerUser,
		workspaceMountPath: workspaceMountPath,
		docker:             cli,
	}, nil
}

// NewDockerRuntimeWithClient creates a DockerRuntime with a provided client (for tests).
func NewDockerRuntimeWithClient(docker DockerClient, containerUser, workspaceMountPath string) *DockerRuntime {
	return &DockerRuntime{
		containerUser:      containerUser,
		workspaceMountPath: workspaceMountPath,
		docker:             docker,
	}
}

// InspectContainer returns the current Docker state for an existing session container.
func (r *DockerRuntime) InspectContainer(
	ctx context.Context,
	meta SessionMetadata,
) (ContainerState, error) {
	resp, err := r.inspectConfig(ctx, meta)
	if err != nil {
		return ContainerState{}, err
	}
	return toContainerState(resp, meta.ContainerName), nil
}

// toContainerState converts a Docker inspect response into the service's smaller state model.
func toContainerState(resp container.InspectResponse, containerName string) ContainerState {
	return ContainerState{
		ID:      resp.ID,
		Name:    containerName,
		Image:   resp.Config.Image,
		State:   resp.State.Status,
		Present: true,
		Running: resp.State.Running,
	}
}

// EnsureContainer creates, replaces, or starts the session container so it is ready for use.
func (r *DockerRuntime) EnsureContainer(
	ctx context.Context,
	meta SessionMetadata,
) (ContainerState, error) {
	current, err := r.inspectConfig(ctx, meta)
	if err != nil {
		if !errors.Is(err, ErrContainerNotFound) {
			return ContainerState{}, err
		}

		// No container exists yet, so create a fresh one for the session.
		if err := r.createContainer(ctx, meta); err != nil {
			return ContainerState{}, err
		}
		if err := r.startContainer(ctx, meta.ContainerName); err != nil {
			return ContainerState{}, err
		}
		return r.reinspect(ctx, meta)
	}

	if !r.matchesExpectedConfig(current, meta) {
		// Recreate containers that drifted from the expected image, user, or mount configuration.
		if err := r.RemoveContainer(ctx, meta); err != nil {
			return ContainerState{}, err
		}
		if err := r.createContainer(ctx, meta); err != nil {
			return ContainerState{}, err
		}
		if err := r.startContainer(ctx, meta.ContainerName); err != nil {
			return ContainerState{}, err
		}
		return r.reinspect(ctx, meta)
	}

	if current.State.Running {
		return toContainerState(current, meta.ContainerName), nil
	}

	if err := r.startContainer(ctx, meta.ContainerName); err != nil {
		// A stopped container may be corrupt; recreate it as a fallback if start fails.
		if removeErr := r.RemoveContainer(ctx, meta); removeErr != nil {
			return ContainerState{}, err
		}
		if err := r.createContainer(ctx, meta); err != nil {
			return ContainerState{}, err
		}
		if err := r.startContainer(ctx, meta.ContainerName); err != nil {
			return ContainerState{}, err
		}
	}

	return r.reinspect(ctx, meta)
}

// reinspect calls inspectConfig once and converts to ContainerState.
func (r *DockerRuntime) reinspect(ctx context.Context, meta SessionMetadata) (ContainerState, error) {
	resp, err := r.inspectConfig(ctx, meta)
	if err != nil {
		return ContainerState{}, err
	}
	return toContainerState(resp, meta.ContainerName), nil
}

// RemoveContainer force-removes the container when it exists.
func (r *DockerRuntime) RemoveContainer(
	ctx context.Context,
	meta SessionMetadata,
) error {
	err := r.docker.ContainerRemove(ctx, meta.ContainerName, container.RemoveOptions{Force: true})
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

// Exec runs a shell command inside the session container and captures bounded output.
func (r *DockerRuntime) Exec(
	ctx context.Context,
	meta SessionMetadata,
	containerCwd string,
	command string,
	timeout time.Duration,
	maxOutputBytes int,
) (ExecResult, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	execResp, err := r.docker.ContainerExecCreate(ctx, meta.ContainerName, container.ExecOptions{
		Cmd:          []string{"bash", "-lc", command},
		WorkingDir:   containerCwd,
		AttachStdout: true,
		AttachStderr: true,
	})
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec create: %w", err)
	}

	attachResp, err := r.docker.ContainerExecAttach(ctx, execResp.ID, container.ExecAttachOptions{})
	if err != nil {
		return ExecResult{}, fmt.Errorf("exec attach: %w", err)
	}
	defer attachResp.Close()

	stdoutBuf := &cappedBuffer{limit: maxOutputBytes}
	stderrBuf := &cappedBuffer{limit: maxOutputBytes}

	streamDone := make(chan error, 1)
	go func() {
		_, err := stdcopy.StdCopy(stdoutBuf, stderrBuf, attachResp.Reader)
		streamDone <- err
	}()

	select {
	case <-streamDone:
		// Stream completed normally.
	case <-ctx.Done():
		// The exec timed out. We still inspect the exec below so callers get the best exit state we can recover.
	}

	result := ExecResult{
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
		TimedOut: errors.Is(ctx.Err(), context.DeadlineExceeded),
	}

	// Use background context since the original may have expired,
	// but we still need to query the exit code.
	inspectResp, err := r.docker.ContainerExecInspect(context.Background(), execResp.ID)
	if err != nil {
		if result.TimedOut {
			result.ExitCode = -1
			return result, nil
		}
		return ExecResult{}, fmt.Errorf("exec inspect: %w", err)
	}

	result.ExitCode = inspectResp.ExitCode

	// Signal detection: exit codes 129–159 map to signals via 128+N convention.
	if result.ExitCode > 128 && result.ExitCode <= 159 {
		sig := syscall.Signal(result.ExitCode - 128)
		result.SignalCode = sig.String()
	}

	if result.TimedOut && result.ExitCode == 0 {
		result.ExitCode = -1
	}

	return result, nil
}

// inspectConfig loads the Docker inspect response and normalizes not-found errors.
func (r *DockerRuntime) inspectConfig(
	ctx context.Context,
	meta SessionMetadata,
) (container.InspectResponse, error) {
	resp, err := r.docker.ContainerInspect(ctx, meta.ContainerName)
	if err != nil {
		if cerrdefs.IsNotFound(err) {
			return container.InspectResponse{}, ErrContainerNotFound
		}
		return container.InspectResponse{}, err
	}
	return resp, nil
}

// createContainer creates a long-lived sleeping container for a session workspace.
func (r *DockerRuntime) createContainer(
	ctx context.Context,
	meta SessionMetadata,
) error {
	_, err := r.docker.ContainerCreate(
		ctx,
		&container.Config{
			Image:      meta.Image,
			User:       r.containerUser,
			WorkingDir: r.workspaceMountPath,
			Cmd:        []string{"sleep", "infinity"},
			Labels: map[string]string{
				"protean.service":    "sandbox-service",
				"protean.session-id": meta.SessionID,
			},
		},
		&container.HostConfig{
			Mounts: []mount.Mount{
				{
					Type:   mount.TypeBind,
					Source: meta.WorkspaceFullPath,
					Target: r.workspaceMountPath,
				},
			},
		},
		nil, // networkingConfig
		nil, // platform
		meta.ContainerName,
	)
	if err != nil {
		return fmt.Errorf("docker create: %w", err)
	}
	return nil
}

// startContainer starts an existing container by name.
func (r *DockerRuntime) startContainer(ctx context.Context, containerName string) error {
	if err := r.docker.ContainerStart(ctx, containerName, container.StartOptions{}); err != nil {
		return fmt.Errorf("docker start: %w", err)
	}
	return nil
}

// matchesExpectedConfig verifies that the container still matches the desired session config.
func (r *DockerRuntime) matchesExpectedConfig(
	resp container.InspectResponse,
	meta SessionMetadata,
) bool {
	if resp.Config.Image != meta.Image {
		return false
	}
	if resp.Config.User != r.containerUser {
		return false
	}
	if resp.Config.WorkingDir != r.workspaceMountPath {
		return false
	}

	// The bind mount source must still point at the session's workspace directory.
	expectedSource := filepath.Clean(meta.WorkspaceFullPath)
	for _, m := range resp.Mounts {
		if m.Destination == r.workspaceMountPath &&
			filepath.Clean(m.Source) == expectedSource {
			return true
		}
	}

	return false
}

// cappedBuffer stores up to limit bytes and records whether content was truncated.
type cappedBuffer struct {
	// limit is the maximum number of bytes to keep in buf.
	limit int
	// buf stores the retained prefix of the stream.
	buf bytes.Buffer
	// truncated reports whether additional bytes were discarded.
	truncated bool
}

// Write appends as much of p as fits in the buffer and discards the rest.
func (b *cappedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 {
		b.truncated = b.truncated || len(p) > 0
		return len(p), nil
	}

	remaining := b.limit - b.buf.Len()
	if remaining > 0 {
		// Only keep the prefix that still fits so callers receive deterministic output.
		chunk := p
		if len(chunk) > remaining {
			chunk = chunk[:remaining]
		}
		if _, err := b.buf.Write(chunk); err != nil {
			return 0, err
		}
	}
	if len(p) > remaining {
		b.truncated = true
	}
	return len(p), nil
}

// String returns the buffered content with a suffix when truncation occurred.
func (b *cappedBuffer) String() string {
	if !b.truncated {
		return b.buf.String()
	}

	return b.buf.String() + "\n[truncated]"
}

// ResolveContainerCwd converts a caller-supplied cwd into a safe in-container path.
func ResolveContainerCwd(root, input string) (string, error) {
	target := input
	if strings.TrimSpace(target) == "" {
		target = "."
	}

	candidate := path.Clean(path.Join(root, target))
	if candidate != root && !strings.HasPrefix(candidate, root+"/") {
		return "", fmt.Errorf("%w: %q", ErrExecCwdEscapesWorkspace, input)
	}

	return candidate, nil
}
