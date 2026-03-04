package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	cerrdefs "github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/pkg/stdcopy"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// mockDockerClient implements DockerClient with function fields for each method.
type mockDockerClient struct {
	inspectFn func(ctx context.Context, id string) (container.InspectResponse, error)
	createFn  func(
		ctx context.Context,
		cfg *container.Config,
		host *container.HostConfig,
		net *network.NetworkingConfig,
		plat *ocispec.Platform,
		name string,
	) (container.CreateResponse, error)
	startFn      func(ctx context.Context, id string, opts container.StartOptions) error
	removeFn     func(ctx context.Context, id string, opts container.RemoveOptions) error
	execCreateFn func(ctx context.Context, id string, opts container.ExecOptions) (container.ExecCreateResponse, error)
	execAttachFn func(ctx context.Context, id string, opts container.ExecAttachOptions) (types.HijackedResponse, error)
	execInspFn   func(ctx context.Context, id string) (container.ExecInspect, error)

	inspectCalls int
	createCalls  int
	startCalls   int
	removeCalls  int
}

func (m *mockDockerClient) ContainerInspect(ctx context.Context, id string) (container.InspectResponse, error) {
	m.inspectCalls++
	if m.inspectFn != nil {
		return m.inspectFn(ctx, id)
	}
	return container.InspectResponse{}, nil
}

func (m *mockDockerClient) ContainerCreate(ctx context.Context, cfg *container.Config, host *container.HostConfig, net *network.NetworkingConfig, plat *ocispec.Platform, name string) (container.CreateResponse, error) {
	m.createCalls++
	if m.createFn != nil {
		return m.createFn(ctx, cfg, host, net, plat, name)
	}
	return container.CreateResponse{}, nil
}

func (m *mockDockerClient) ContainerStart(ctx context.Context, id string, opts container.StartOptions) error {
	m.startCalls++
	if m.startFn != nil {
		return m.startFn(ctx, id, opts)
	}
	return nil
}

func (m *mockDockerClient) ContainerRemove(ctx context.Context, id string, opts container.RemoveOptions) error {
	m.removeCalls++
	if m.removeFn != nil {
		return m.removeFn(ctx, id, opts)
	}
	return nil
}

func (m *mockDockerClient) ContainerExecCreate(ctx context.Context, id string, opts container.ExecOptions) (container.ExecCreateResponse, error) {
	if m.execCreateFn != nil {
		return m.execCreateFn(ctx, id, opts)
	}
	return container.ExecCreateResponse{ID: "exec-1"}, nil
}

func (m *mockDockerClient) ContainerExecAttach(ctx context.Context, id string, opts container.ExecAttachOptions) (types.HijackedResponse, error) {
	if m.execAttachFn != nil {
		return m.execAttachFn(ctx, id, opts)
	}
	return types.HijackedResponse{}, nil
}

func (m *mockDockerClient) ContainerExecInspect(ctx context.Context, id string) (container.ExecInspect, error) {
	if m.execInspFn != nil {
		return m.execInspFn(ctx, id)
	}
	return container.ExecInspect{}, nil
}

// dockerMultiplexedStream creates a properly framed Docker multiplexed stream
// with the given stdout and stderr content, using stdcopy.NewStdWriter.
func dockerMultiplexedStream(stdoutData, stderrData string) io.Reader {
	var buf bytes.Buffer
	if stdoutData != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stdout)
		w.Write([]byte(stdoutData))
	}
	if stderrData != "" {
		w := stdcopy.NewStdWriter(&buf, stdcopy.Stderr)
		w.Write([]byte(stderrData))
	}
	return &buf
}

// nopConn is a minimal net.Conn that does nothing, for test HijackedResponse.
type nopConn struct{ io.Reader }

func (nopConn) Write(b []byte) (int, error)      { return len(b), nil }
func (nopConn) Close() error                     { return nil }
func (nopConn) LocalAddr() net.Addr              { return nil }
func (nopConn) RemoteAddr() net.Addr             { return nil }
func (nopConn) SetDeadline(time.Time) error      { return nil }
func (nopConn) SetReadDeadline(time.Time) error  { return nil }
func (nopConn) SetWriteDeadline(time.Time) error { return nil }

// hijackedResponse creates a types.HijackedResponse with a multiplexed stream.
func hijackedResponse(stdoutData, stderrData string) types.HijackedResponse {
	reader := dockerMultiplexedStream(stdoutData, stderrData)
	conn := nopConn{Reader: reader}
	return types.HijackedResponse{
		Reader: bufio.NewReader(conn),
		Conn:   conn,
	}
}

func runningInspectResponse(id, image, user, workDir string, mounts []container.MountPoint) container.InspectResponse {
	return container.InspectResponse{
		ContainerJSONBase: &container.ContainerJSONBase{
			ID: id,
			State: &container.State{
				Status:  "running",
				Running: true,
			},
		},
		Config: &container.Config{
			Image:      image,
			User:       user,
			WorkingDir: workDir,
		},
		Mounts: mounts,
	}
}

func TestInspectContainerParsesPayload(t *testing.T) {
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return runningInspectResponse("container-1", "image:1", "1000:1000", "/workspace", []container.MountPoint{
				{Source: "/tmp/workspace", Destination: "/workspace"},
			}), nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	state, err := runtime.InspectContainer(context.Background(), SessionMetadata{
		ContainerName:     "sandbox-1",
		WorkspaceFullPath: "/tmp/workspace",
		Image:             "image:1",
	})
	if err != nil {
		t.Fatalf("InspectContainer failed: %v", err)
	}

	if !state.Present || !state.Running || state.ID != "container-1" {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestInspectContainerMapsNotFound(t *testing.T) {
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return container.InspectResponse{}, cerrdefs.ErrNotFound
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	_, err := runtime.InspectContainer(context.Background(), SessionMetadata{
		ContainerName: "sandbox-1",
	})
	if !errors.Is(err, ErrContainerNotFound) {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
}

func TestInspectContainerPreservesOtherErrors(t *testing.T) {
	boom := errors.New("daemon unavailable")
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			return container.InspectResponse{}, boom
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	_, err := runtime.InspectContainer(context.Background(), SessionMetadata{
		ContainerName: "sandbox-1",
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected original error, got %v", err)
	}
}

func TestEnsureContainerCreatesWhenMissing(t *testing.T) {
	inspectCall := 0
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			inspectCall++
			if inspectCall == 1 {
				return container.InspectResponse{}, cerrdefs.ErrNotFound
			}
			return runningInspectResponse("container-1", "image:1", "1000:1000", "/workspace", []container.MountPoint{
				{Source: "/tmp/workspace", Destination: "/workspace"},
			}), nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	state, err := runtime.EnsureContainer(context.Background(), validMetadata())
	if err != nil {
		t.Fatalf("EnsureContainer failed: %v", err)
	}
	if !state.Running {
		t.Fatalf("expected running container, got %+v", state)
	}

	// Verify call sequence: inspect(not found), create, start, inspect(ok)
	if mock.inspectCalls != 2 {
		t.Fatalf("expected 2 inspect calls, got %d", mock.inspectCalls)
	}
	if mock.createCalls != 1 {
		t.Fatalf("expected 1 create call, got %d", mock.createCalls)
	}
	if mock.startCalls != 1 {
		t.Fatalf("expected 1 start call, got %d", mock.startCalls)
	}
}

func TestEnsureContainerRecreatesOnConfigMismatch(t *testing.T) {
	inspectCall := 0
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			inspectCall++
			if inspectCall == 1 {
				// Wrong image
				return runningInspectResponse("container-1", "wrong:1", "1000:1000", "/workspace", []container.MountPoint{
					{Source: "/tmp/workspace", Destination: "/workspace"},
				}), nil
			}
			return runningInspectResponse("container-2", "image:1", "1000:1000", "/workspace", []container.MountPoint{
				{Source: "/tmp/workspace", Destination: "/workspace"},
			}), nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	state, err := runtime.EnsureContainer(context.Background(), validMetadata())
	if err != nil {
		t.Fatalf("EnsureContainer failed: %v", err)
	}
	if state.ID != "container-2" {
		t.Fatalf("expected recreated container, got %+v", state)
	}

	if mock.removeCalls != 1 {
		t.Fatalf("expected 1 remove call, got %d", mock.removeCalls)
	}
}

func TestEnsureContainerRecreatesWhenRestartFails(t *testing.T) {
	inspectCall := 0
	startCall := 0
	mock := &mockDockerClient{
		inspectFn: func(_ context.Context, _ string) (container.InspectResponse, error) {
			inspectCall++
			if inspectCall == 1 {
				// Exited container with matching config
				return container.InspectResponse{
					ContainerJSONBase: &container.ContainerJSONBase{
						ID: "container-1",
						State: &container.State{
							Status:  "exited",
							Running: false,
						},
					},
					Config: &container.Config{
						Image:      "image:1",
						User:       "1000:1000",
						WorkingDir: "/workspace",
					},
					Mounts: []container.MountPoint{
						{Source: "/tmp/workspace", Destination: "/workspace"},
					},
				}, nil
			}
			return runningInspectResponse("container-2", "image:1", "1000:1000", "/workspace", []container.MountPoint{
				{Source: "/tmp/workspace", Destination: "/workspace"},
			}), nil
		},
		startFn: func(_ context.Context, _ string, _ container.StartOptions) error {
			startCall++
			if startCall == 1 {
				return errors.New("cannot start")
			}
			return nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	state, err := runtime.EnsureContainer(context.Background(), validMetadata())
	if err != nil {
		t.Fatalf("EnsureContainer failed: %v", err)
	}
	if state.ID != "container-2" {
		t.Fatalf("expected recreated container, got %+v", state)
	}
}

func TestExecReturnsOutput(t *testing.T) {
	mock := &mockDockerClient{
		execAttachFn: func(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
			return hijackedResponse("hello world", ""), nil
		},
		execInspFn: func(_ context.Context, _ string) (container.ExecInspect, error) {
			return container.ExecInspect{ExitCode: 0}, nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	result, err := runtime.Exec(context.Background(), validMetadata(), "/workspace", "echo hello", time.Second, 1024)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.Stdout != "hello world" || result.ExitCode != 0 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecReturnsExitCode(t *testing.T) {
	mock := &mockDockerClient{
		execAttachFn: func(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
			return hijackedResponse("", ""), nil
		},
		execInspFn: func(_ context.Context, _ string) (container.ExecInspect, error) {
			return container.ExecInspect{ExitCode: 7}, nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	result, err := runtime.Exec(context.Background(), validMetadata(), "/workspace", "false", time.Second, 64)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.ExitCode != 7 || result.TimedOut {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestExecReturnsSignalCode(t *testing.T) {
	mock := &mockDockerClient{
		execAttachFn: func(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
			return hijackedResponse("", ""), nil
		},
		execInspFn: func(_ context.Context, _ string) (container.ExecInspect, error) {
			// 128 + 15 (SIGTERM) = 143
			return container.ExecInspect{ExitCode: 143}, nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	result, err := runtime.Exec(context.Background(), validMetadata(), "/workspace", "trap", time.Second, 64)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if result.SignalCode == "" {
		t.Fatalf("expected signal code, got %+v", result)
	}
	if result.SignalCode != "terminated" {
		t.Fatalf("expected 'terminated', got %q", result.SignalCode)
	}
}

func TestExecReturnsTimeoutResult(t *testing.T) {
	mock := &mockDockerClient{
		execAttachFn: func(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
			// Return a reader that blocks until context is cancelled
			pr, _ := io.Pipe()
			conn := nopConn{Reader: pr}
			return types.HijackedResponse{
				Reader: bufio.NewReader(conn),
				Conn:   conn,
			}, nil
		},
		execInspFn: func(_ context.Context, _ string) (container.ExecInspect, error) {
			// After timeout, exec inspect might still show running (exit code 0)
			return container.ExecInspect{ExitCode: 0}, nil
		},
	}
	runtime := NewDockerRuntimeWithClient(mock, "1000:1000", "/workspace")

	result, err := runtime.Exec(
		context.Background(),
		validMetadata(),
		"/workspace",
		"sleep 10",
		5*time.Millisecond,
		64,
	)
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}
	if !result.TimedOut || result.ExitCode != -1 {
		t.Fatalf("expected timeout result, got %+v", result)
	}
}

func TestCappedBufferRespectsLimit(t *testing.T) {
	buffer := &cappedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("abcd")); err != nil {
		t.Fatalf("write exact limit: %v", err)
	}
	if got := buffer.String(); got != "abcd" {
		t.Fatalf("unexpected exact output: %q", got)
	}

	buffer = &cappedBuffer{limit: 4}
	if _, err := buffer.Write([]byte("abcdef")); err != nil {
		t.Fatalf("write over limit: %v", err)
	}
	if got := buffer.String(); got != "abcd\n[truncated]" {
		t.Fatalf("unexpected truncated output: %q", got)
	}

	buffer = &cappedBuffer{limit: 0}
	if _, err := buffer.Write([]byte("abcdef")); err != nil {
		t.Fatalf("write zero limit: %v", err)
	}
	if got := buffer.String(); got != "\n[truncated]" {
		t.Fatalf("unexpected zero-limit output: %q", got)
	}
}

func TestResolveContainerCwd(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{name: "empty", input: "", want: "/workspace"},
		{name: "nested", input: "nested", want: "/workspace/nested"},
		{name: "root", input: ".", want: "/workspace"},
		{name: "escape", input: "../..", wantErr: ErrExecCwdEscapesWorkspace},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveContainerCwd("/workspace", tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveContainerCwd failed: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func validMetadata() SessionMetadata {
	return SessionMetadata{
		SessionID:          "session-1",
		WorkspaceFullPath:  "/tmp/workspace",
		WorkspaceMountPath: "/workspace",
		ContainerName:      "protean-sandbox-1",
		Image:              "image:1",
	}
}
