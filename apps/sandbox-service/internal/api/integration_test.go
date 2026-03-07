//go:build integration

package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/protean/sandbox-service/internal/sandbox"
)

func TestSandboxLifecycleWithDocker(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}

	image := os.Getenv("SANDBOX_TEST_IMAGE")
	if image == "" {
		t.Skip("SANDBOX_TEST_IMAGE is not set")
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("chmod workspace root: %v", err)
	}
	rt, err := sandbox.NewDockerRuntime("1000:1000", "/workspace")
	if err != nil {
		t.Fatalf("new docker runtime: %v", err)
	}
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox-it",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       image,
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, rt)

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

	writeReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID+"/files/write",
		bytes.NewBufferString(`{"path":"note.txt","content":"hello"}`),
	)
	writeReq.Header.Set("Authorization", "Bearer token")
	writeReq.Header.Set("Content-Type", "application/json")
	writeRec := httptest.NewRecorder()
	handler.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", writeRec.Code)
	}

	execReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+createEnvelope.Data.SessionID+"/exec",
		bytes.NewBufferString(`{"command":"cat note.txt"}`),
	)
	execReq.Header.Set("Authorization", "Bearer token")
	execReq.Header.Set("Content-Type", "application/json")
	execRec := httptest.NewRecorder()
	handler.ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", execRec.Code)
	}

	if _, err := os.Stat(filepath.Join(createEnvelope.Data.WorkspaceFullPath, "note.txt")); err != nil {
		t.Fatalf("expected note.txt to persist: %v", err)
	}
}

func TestInvalidMetadataReturnsNotFoundWithDocker(t *testing.T) {
	handler, session := newDockerTestServerWithSession(t)
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

func TestStatusReportsMissingContainerAndExecRecreatesIt(t *testing.T) {
	handler, session := newDockerTestServerWithSession(t)
	removeDockerContainer(t, session.ContainerName)

	statusReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+session.SessionID,
		nil,
	)
	statusReq.Header.Set("Authorization", "Bearer token")
	statusRec := httptest.NewRecorder()
	handler.ServeHTTP(statusRec, statusReq)
	if statusRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", statusRec.Code)
	}

	var statusEnvelope struct {
		OK   bool                         `json:"ok"`
		Data sandbox.SandboxSessionStatus `json:"data"`
	}
	if err := json.Unmarshal(statusRec.Body.Bytes(), &statusEnvelope); err != nil {
		t.Fatalf("decode status response: %v", err)
	}
	if statusEnvelope.Data.ContainerPresent || statusEnvelope.Data.ContainerRunning {
		t.Fatalf("expected missing container, got %+v", statusEnvelope.Data)
	}

	execReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/exec",
		bytes.NewBufferString(`{"command":"echo ok"}`),
	)
	execReq.Header.Set("Authorization", "Bearer token")
	execReq.Header.Set("Content-Type", "application/json")
	execRec := httptest.NewRecorder()
	handler.ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", execRec.Code)
	}
}

func TestLargeTextReadReturnsBadRequest(t *testing.T) {
	handler, session := newDockerTestServerWithSession(t)
	content := bytes.Repeat([]byte("a"), int(maxTextReadBytes)+1)
	if err := os.WriteFile(filepath.Join(session.WorkspaceFullPath, "large.txt"), content, 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	readReq := httptest.NewRequest(
		http.MethodGet,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/files/read?path=large.txt",
		nil,
	)
	readReq.Header.Set("Authorization", "Bearer token")
	readRec := httptest.NewRecorder()
	handler.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", readRec.Code)
	}
}

func TestDirectoryRenamePreservesDescendants(t *testing.T) {
	handler, session := newDockerTestServerWithSession(t)
	if err := os.MkdirAll(filepath.Join(session.WorkspaceFullPath, "dir"), 0o755); err != nil {
		t.Fatalf("mkdir dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(session.WorkspaceFullPath, "dir", "child.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatalf("write child: %v", err)
	}

	renameReq := httptest.NewRequest(
		http.MethodPatch,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/files/rename",
		bytes.NewBufferString(`{"path":"dir","newPath":"moved"}`),
	)
	renameReq.Header.Set("Authorization", "Bearer token")
	renameReq.Header.Set("Content-Type", "application/json")
	renameRec := httptest.NewRecorder()
	handler.ServeHTTP(renameRec, renameReq)
	if renameRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", renameRec.Code)
	}

	execReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/sandbox/sessions/"+session.SessionID+"/exec",
		bytes.NewBufferString(`{"command":"cat moved/child.txt"}`),
	)
	execReq.Header.Set("Authorization", "Bearer token")
	execReq.Header.Set("Content-Type", "application/json")
	execRec := httptest.NewRecorder()
	handler.ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", execRec.Code)
	}
}

func newDockerTestServerWithSession(t *testing.T) (*Server, sandbox.SandboxSession) {
	t.Helper()

	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}

	image := os.Getenv("SANDBOX_TEST_IMAGE")
	if image == "" {
		t.Skip("SANDBOX_TEST_IMAGE is not set")
	}

	root := t.TempDir()
	if err := os.Chmod(root, 0o755); err != nil {
		t.Fatalf("chmod workspace root: %v", err)
	}

	rt, err := sandbox.NewDockerRuntime("1000:1000", "/workspace")
	if err != nil {
		t.Fatalf("new docker runtime: %v", err)
	}
	svc := sandbox.NewService(sandbox.ServiceConfig{
		WorkspaceBase:      root,
		ContainerPrefix:    "protean-sandbox-it",
		WorkspaceMountPath: "/workspace",
		DefaultImage:       image,
		Logger:             testLogger(),
		ExecDefaultTimeout: 30 * time.Second,
		ExecMaxTimeout:     5 * time.Minute,
		ExecMaxOutputBytes: 65536,
	}, rt)

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

	return handler, createEnvelope.Data
}

func removeDockerContainer(t *testing.T, containerName string) {
	t.Helper()

	cmd := exec.Command("docker", "rm", "-f", containerName)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("remove docker container: %v (%s)", err, string(output))
	}
}
