package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/protean/sandbox-service/internal/sandbox"
)

type fakeRuntime struct {
	inspectErr error
	ensureErr  error
	removeErr  error
	execErr    error
	execDelay  time.Duration
}

func (f *fakeRuntime) InspectContainer(
	_ context.Context,
	meta sandbox.SessionMetadata,
) (sandbox.ContainerState, error) {
	if f.inspectErr != nil {
		return sandbox.ContainerState{}, f.inspectErr
	}
	return sandbox.ContainerState{
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
	meta sandbox.SessionMetadata,
) (sandbox.ContainerState, error) {
	if f.ensureErr != nil {
		return sandbox.ContainerState{}, f.ensureErr
	}
	return sandbox.ContainerState{
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
	_ sandbox.SessionMetadata,
) error {
	return f.removeErr
}

func (f *fakeRuntime) Exec(
	_ context.Context,
	_ sandbox.SessionMetadata,
	_ string,
	command string,
	_ time.Duration,
	_ int,
) (sandbox.ExecResult, error) {
	if f.execDelay > 0 {
		time.Sleep(f.execDelay)
	}
	if f.execErr != nil {
		return sandbox.ExecResult{}, f.execErr
	}
	return sandbox.ExecResult{
		ExitCode: 0,
		Stdout:   command,
	}, nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestExecRejectsUnknownFields(t *testing.T) {
	handler, sessionID := newTestServerWithSession(t, &fakeRuntime{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+sessionID+"/exec",
		bytes.NewBufferString(`{"command":"pwd","extra":true}`),
	)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestExecRejectsTrailingJSON(t *testing.T) {
	handler, sessionID := newTestServerWithSession(t, &fakeRuntime{})

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+sessionID+"/exec",
		bytes.NewBufferString(`{"command":"pwd"}{"extra":true}`),
	)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestWriteRejectsOversizedJSONBody(t *testing.T) {
	handler, sessionID := newTestServerWithSession(t, &fakeRuntime{})
	content := strings.Repeat("a", int(maxJSONBodyBytes))
	body := `{"path":"note.txt","content":"` + content + `"}`

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+sessionID+"/files/write",
		bytes.NewBufferString(body),
	)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestReadRejectsOversizedTextFile(t *testing.T) {
	handler, session := newTestServerWithSessionDetails(t, &fakeRuntime{})
	content := bytes.Repeat([]byte("a"), int(maxTextReadBytes)+1)
	if err := os.WriteFile(filepath.Join(session.WorkspaceFullPath, "large.txt"), content, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/files/read?path=large.txt",
		nil,
	)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestReadBinaryRejectsOversizedFile(t *testing.T) {
	handler, session := newTestServerWithSessionDetails(t, &fakeRuntime{})
	content := bytes.Repeat([]byte("a"), int(maxBinaryReadBytes)+1)
	if err := os.WriteFile(filepath.Join(session.WorkspaceFullPath, "large.bin"), content, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/files/read-binary?path=large.bin",
		nil,
	)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestInvalidMetadataMapsToNotFound(t *testing.T) {
	handler, session := newTestServerWithSessionDetails(t, &fakeRuntime{})
	metadataPath := filepath.Join(session.WorkspaceFullPath, ".sandbox-session.json")
	if err := os.WriteFile(metadataPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("corrupt metadata: %v", err)
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+session.SessionID,
		nil,
	)
	req.Header.Set("Authorization", "Bearer token")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestInternalRuntimeErrorsDoNotLeak(t *testing.T) {
	runtime := &fakeRuntime{}
	handler, sessionID := newTestServerWithSession(t, runtime)
	runtime.execErr = errors.New("docker exploded: /tmp/secret")

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+sessionID+"/exec",
		bytes.NewBufferString(`{"command":"pwd"}`),
	)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "/tmp/secret") {
		t.Fatalf("expected internal details to be hidden, got %s", rec.Body.String())
	}
}

func TestRenameRejectsWorkspaceRootDestination(t *testing.T) {
	handler, sessionID := newTestServerWithSession(t, &fakeRuntime{})

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/sandbox/sessions/"+sessionID+"/files/rename",
		bytes.NewBufferString(`{"path":"a.txt","newPath":"/"}`),
	)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", rec.Code)
	}
}

func TestOverlappingRenameAndWriteRemainConsistent(t *testing.T) {
	handler, session := newTestServerWithSessionDetails(t, &fakeRuntime{})
	oldDir := filepath.Join(session.WorkspaceFullPath, "dir")
	newDir := filepath.Join(session.WorkspaceFullPath, "renamed")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("mkdir old dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "seed.txt"), []byte("seed"), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		req := httptest.NewRequest(
			http.MethodPatch,
			"/api/v1/sandbox/sessions/"+session.SessionID+"/files/rename",
			bytes.NewBufferString(`{"path":"dir","newPath":"renamed"}`),
		)
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	go func() {
		defer wg.Done()
		req := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/sandbox/sessions/"+session.SessionID+"/files/write",
			bytes.NewBufferString(`{"path":"dir/child.txt","content":"ok"}`),
		)
		req.Header.Set("Authorization", "Bearer token")
		req.Header.Set("Content-Type", "application/json")
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}()

	wg.Wait()

	oldChild := filepath.Join(oldDir, "child.txt")
	newChild := filepath.Join(newDir, "child.txt")
	_, oldErr := os.Stat(oldChild)
	_, newErr := os.Stat(newChild)
	oldExists := oldErr == nil
	newExists := newErr == nil
	if oldExists == newExists {
		t.Fatal("expected child file to exist in exactly one location")
	}
}

func TestUnauthorizedRequestsAreRejected(t *testing.T) {
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      t.TempDir(),
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	handler := New(Config{
		ServiceTokens: map[string]string{"token": "test"},
		Service:       svc,
		Logger:        testLogger(),
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/sessions", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestSessionLifecycleEndpoints(t *testing.T) {
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      t.TempDir(),
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	handler := New(Config{
		ServiceTokens: map[string]string{"token": "test"},
		Service:       svc,
		Logger:        testLogger(),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/sessions", nil)
	createReq.Header.Set("Authorization", "Bearer token")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", createRec.Code)
	}

	var createEnvelope struct {
		OK   bool                   `json:"ok"`
		Data sandbox.SandboxSession `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	getReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID,
		nil,
	)
	getReq.Header.Set("Authorization", "Bearer token")
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	execBody := bytes.NewBufferString(`{"command":"pwd"}`)
	execReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID+"/exec",
		execBody,
	)
	execReq.Header.Set("Authorization", "Bearer token")
	execReq.Header.Set("Content-Type", "application/json")
	execRec := httptest.NewRecorder()
	handler.ServeHTTP(execRec, execReq)

	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", execRec.Code)
	}
}

func TestReservedMetadataPathIsBlocked(t *testing.T) {
	root := t.TempDir()
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	handler := New(Config{
		ServiceTokens: map[string]string{"token": "test"},
		Service:       svc,
		Logger:        testLogger(),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/sessions", nil)
	createReq.Header.Set("Authorization", "Bearer token")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var createEnvelope struct {
		OK   bool                   `json:"ok"`
		Data sandbox.SandboxSession `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	readReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID+"/files/read?path=.sandbox-session.json",
		nil,
	)
	readReq.Header.Set("Authorization", "Bearer token")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", readRec.Code)
	}
}

func TestSymlinkAccessIsBlocked(t *testing.T) {
	root := t.TempDir()
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, &fakeRuntime{})

	handler := New(Config{
		ServiceTokens: map[string]string{"token": "test"},
		Service:       svc,
		Logger:        testLogger(),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/sessions", nil)
	createReq.Header.Set("Authorization", "Bearer token")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	var createEnvelope struct {
		OK   bool                   `json:"ok"`
		Data sandbox.SandboxSession `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	targetPath := filepath.Join(createEnvelope.Data.WorkspaceFullPath, "real.txt")
	if err := os.WriteFile(targetPath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}
	if err := os.Symlink(targetPath, filepath.Join(createEnvelope.Data.WorkspaceFullPath, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	readReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID+"/files/read?path=link.txt",
		nil,
	)
	readReq.Header.Set("Authorization", "Bearer token")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)

	if readRec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", readRec.Code)
	}
}

func newTestServerWithSession(
	t *testing.T,
	runtime *fakeRuntime,
) (*Server, string) {
	t.Helper()

	handler, session := newTestServerWithSessionDetails(t, runtime)
	return handler, session.SessionID
}

func newTestServerWithSessionDetails(
	t *testing.T,
	runtime *fakeRuntime,
) (*Server, sandbox.SandboxSession) {
	t.Helper()

	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      t.TempDir(),
		ContainerPrefix:    "protean-sandbox",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       "protean-sandbox:1",
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, runtime)

	handler := New(Config{
		ServiceTokens: map[string]string{"token": "test"},
		Service:       svc,
		Logger:        testLogger(),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/sandbox/sessions", nil)
	createReq.Header.Set("Authorization", "Bearer token")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusOK {
		t.Fatalf("expected create 200, got %d", createRec.Code)
	}

	var createEnvelope struct {
		OK   bool                   `json:"ok"`
		Data sandbox.SandboxSession `json:"data"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &createEnvelope); err != nil {
		t.Fatalf("decode create response: %v", err)
	}

	return handler, createEnvelope.Data
}
