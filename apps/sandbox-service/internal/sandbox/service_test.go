package sandbox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeRuntime struct {
	ensureCalls int
	removeCalls int
	ensureErr   error
	lastExec    struct {
		cwd     string
		command string
	}
	inspectErr error
}

func (f *fakeRuntime) InspectContainer(
	_ context.Context,
	meta SessionMetadata,
) (ContainerState, error) {
	if f.inspectErr != nil {
		return ContainerState{}, f.inspectErr
	}

	return ContainerState{
		ID:      "container-1",
		Name:    meta.ContainerName,
		Image:   meta.Image,
		State:   "running",
		Present: true,
		Running: true,
	}, nil
}

func (f *fakeRuntime) EnsureContainer(
	_ context.Context,
	meta SessionMetadata,
) (ContainerState, error) {
	f.ensureCalls++
	if f.ensureErr != nil {
		return ContainerState{}, f.ensureErr
	}
	return ContainerState{
		ID:      "container-1",
		Name:    meta.ContainerName,
		Image:   meta.Image,
		State:   "running",
		Present: true,
		Running: true,
	}, nil
}

func (f *fakeRuntime) RemoveContainer(
	_ context.Context,
	_ SessionMetadata,
) error {
	f.removeCalls++
	return nil
}

func (f *fakeRuntime) Exec(
	_ context.Context,
	_ SessionMetadata,
	containerCwd string,
	command string,
	_ time.Duration,
	_ int,
) (ExecResult, error) {
	f.lastExec.cwd = containerCwd
	f.lastExec.command = command
	return ExecResult{
		ExitCode: 0,
		Stdout:   "ok",
	}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCreateSessionWritesMetadata(t *testing.T) {
	root := t.TempDir()
	runtime := &fakeRuntime{}
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, runtime)

	session, err := service.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	if session.WorkspaceFullPath == "" {
		t.Fatal("expected workspace full path")
	}

	metadataPath := filepath.Join(session.WorkspaceFullPath, MetadataFilename)
	if _, err := os.Stat(metadataPath); err != nil {
		t.Fatalf("expected metadata file, got %v", err)
	}
	if runtime.ensureCalls != 1 {
		t.Fatalf("expected one ensure call, got %d", runtime.ensureCalls)
	}
}

func TestGetSessionStatusWithoutContainerStillUsesMetadata(t *testing.T) {
	root := t.TempDir()
	runtime := &fakeRuntime{inspectErr: ErrContainerNotFound}
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, runtime)

	session, err := service.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	status, err := service.GetSessionStatus(context.Background(), session.SessionID)
	if err != nil {
		t.Fatalf("GetSessionStatus failed: %v", err)
	}

	if !status.Exists || !status.WorkspaceReady {
		t.Fatal("expected existing session")
	}
	if status.ContainerPresent || status.ContainerRunning {
		t.Fatal("expected missing container state")
	}
}

func TestExecEnsuresContainerAndNormalizesCwd(t *testing.T) {
	root := t.TempDir()
	runtime := &fakeRuntime{}
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, runtime)

	session, err := service.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}
	runtime.ensureCalls = 0

	_, err = service.Exec(context.Background(), session.SessionID, ExecRequest{
		Command: "pwd",
		Cwd:     "nested",
	})
	if err != nil {
		t.Fatalf("Exec failed: %v", err)
	}

	if runtime.ensureCalls != 1 {
		t.Fatalf("expected ensure to run for exec, got %d", runtime.ensureCalls)
	}
	if runtime.lastExec.cwd != "/workspace/nested" {
		t.Fatalf("unexpected cwd: %s", runtime.lastExec.cwd)
	}
}

func TestLoadMetadataRejectsTamperedWorkspacePath(t *testing.T) {
	root := t.TempDir()
	runtime := &fakeRuntime{}
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, runtime)

	session, err := service.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	metadataPath := filepath.Join(session.WorkspaceFullPath, MetadataFilename)
	badMetadata := []byte(`{
  "sessionId": "` + session.SessionID + `",
  "createdAt": "2026-03-01T00:00:00Z",
  "workspaceFullPath": "/tmp/elsewhere",
  "workspaceMountPath": "/workspace",
  "containerName": "protean-sandbox-` + session.SessionID + `",
  "image": "protean-sandbox:1"
}`)
	if err := os.WriteFile(metadataPath, badMetadata, 0644); err != nil {
		t.Fatalf("write bad metadata: %v", err)
	}

	if _, err := service.LoadMetadata(session.SessionID); !errors.Is(err, ErrInvalidSessionMetadata) {
		t.Fatalf("expected ErrInvalidSessionMetadata, got %v", err)
	}
}

func TestLoadMetadataRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	session, err := service.CreateSession(context.Background())
	if err != nil {
		t.Fatalf("CreateSession failed: %v", err)
	}

	metadataPath := filepath.Join(session.WorkspaceFullPath, MetadataFilename)
	if err := os.WriteFile(metadataPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write malformed metadata: %v", err)
	}

	if _, err := service.LoadMetadata(session.SessionID); !errors.Is(err, ErrInvalidSessionMetadata) {
		t.Fatalf("expected ErrInvalidSessionMetadata, got %v", err)
	}
}

func TestLoadMetadataMissingReturnsNotFound(t *testing.T) {
	root := t.TempDir()
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	if _, err := service.LoadMetadata("01KJN51BVM9V3QRR8SPDG8Q3KQ"); !errors.Is(err, ErrSessionNotFound) {
		t.Fatalf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestCreateSessionRollsBackWorkspaceOnEnsureFailure(t *testing.T) {
	root := t.TempDir()
	logs := &bytes.Buffer{}
	service := NewService(ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             slog.New(slog.NewTextHandler(logs, nil)),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{ensureErr: errors.New("boom")})

	_, err := service.CreateSession(context.Background())
	if err == nil {
		t.Fatal("expected CreateSession to fail")
	}

	entries, readErr := os.ReadDir(root)
	if readErr != nil {
		t.Fatalf("read root: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("expected workspace rollback, found %d entries", len(entries))
	}
	if !strings.Contains(logs.String(), "sandbox session create failed") {
		t.Fatalf("expected create failure log, got %q", logs.String())
	}
}

func TestNormalizeTimeoutCapsValues(t *testing.T) {
	service := NewService(ServiceConfig{
		WorkspaceBase:      t.TempDir(),
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	if got := service.normalizeTimeout(0); got != 30*time.Second {
		t.Fatalf("expected default timeout, got %s", got)
	}
	if got := service.normalizeTimeout(int((10 * time.Minute).Milliseconds())); got != 5*time.Minute {
		t.Fatalf("expected capped timeout, got %s", got)
	}
}

func TestExecRejectsEmptyCommand(t *testing.T) {
	service := NewService(ServiceConfig{
		WorkspaceBase:      t.TempDir(),
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	_, err := service.Exec(context.Background(), "01KJN51BVM9V3QRR8SPDG8Q3KQ", ExecRequest{})
	if !errors.Is(err, ErrExecInvalidRequest) {
		t.Fatalf("expected ErrExecInvalidRequest, got %v", err)
	}
}
