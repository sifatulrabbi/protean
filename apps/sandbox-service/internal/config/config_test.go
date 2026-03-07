package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRequiresWorkspaceBase(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_WORKSPACE_BASE", "")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SANDBOX_WORKSPACE_BASE is required") {
		t.Fatalf("expected workspace base error, got %v", err)
	}
}

func TestLoadRejectsRelativeWorkspaceBase(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_WORKSPACE_BASE", "relative/path")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("expected absolute path error, got %v", err)
	}
}

func TestLoadRejectsInvalidInteger(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_EXEC_MAX_OUTPUT_BYTES", "abc")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SANDBOX_EXEC_MAX_OUTPUT_BYTES must be an integer") {
		t.Fatalf("expected integer error, got %v", err)
	}
}

func TestLoadRejectsDefaultTimeoutGreaterThanMax(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_EXEC_DEFAULT_TIMEOUT_MS", "5000")
	t.Setenv("SANDBOX_EXEC_MAX_TIMEOUT_MS", "1000")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must be less than or equal") {
		t.Fatalf("expected timeout ordering error, got %v", err)
	}
}

func TestLoadRejectsRelativeContainerWorkdir(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_CONTAINER_WORKDIR", "workspace")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "SANDBOX_CONTAINER_WORKDIR must be an absolute path") {
		t.Fatalf("expected workdir error, got %v", err)
	}
}

func TestLoadRejectsDuplicateToken(t *testing.T) {
	setBaseEnv(t)
	t.Setenv("SANDBOX_SERVICE_TOKENS", "svc1:token,svc2:token")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "duplicate service token") {
		t.Fatalf("expected duplicate token error, got %v", err)
	}
}

func TestLoadTrimsWhitespace(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	t.Setenv("SANDBOX_WORKSPACE_BASE", "  "+root+"  ")
	t.Setenv("SANDBOX_SERVICE_TOKENS", "svc:token")
	t.Setenv("SANDBOX_DEFAULT_IMAGE", "  image:1  ")
	t.Setenv("SANDBOX_CONTAINER_PREFIX", "  prefix  ")
	t.Setenv("SANDBOX_CONTAINER_WORKDIR", "  /workspace  ")
	t.Setenv("SANDBOX_CONTAINER_USER", " 1000:1000 ")
	t.Setenv("SANDBOX_EXEC_DEFAULT_TIMEOUT_MS", "30000")
	t.Setenv("SANDBOX_EXEC_MAX_TIMEOUT_MS", "300000")
	t.Setenv("SANDBOX_EXEC_MAX_OUTPUT_BYTES", "65536")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.WorkspaceBase != root {
		t.Fatalf("expected trimmed workspace base, got %q", cfg.WorkspaceBase)
	}
	if cfg.DefaultImage != "image:1" {
		t.Fatalf("expected trimmed image, got %q", cfg.DefaultImage)
	}
	if cfg.ContainerPrefix != "prefix" {
		t.Fatalf("expected trimmed prefix, got %q", cfg.ContainerPrefix)
	}
	if cfg.ContainerWorkdir != "/workspace" {
		t.Fatalf("expected trimmed workdir, got %q", cfg.ContainerWorkdir)
	}
	if cfg.ContainerUser != "1000:1000" {
		t.Fatalf("expected trimmed user, got %q", cfg.ContainerUser)
	}
}

func setBaseEnv(t *testing.T) {
	t.Helper()

	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	t.Setenv("SANDBOX_WORKSPACE_BASE", root)
	t.Setenv("SANDBOX_SERVICE_TOKENS", "svc:token")
	t.Setenv("SANDBOX_DEFAULT_IMAGE", "image:1")
	t.Setenv("SANDBOX_CONTAINER_PREFIX", "protean-sandbox")
	t.Setenv("SANDBOX_CONTAINER_WORKDIR", "/workspace")
	t.Setenv("SANDBOX_CONTAINER_USER", "1000:1000")
	t.Setenv("SANDBOX_EXEC_DEFAULT_TIMEOUT_MS", "30000")
	t.Setenv("SANDBOX_EXEC_MAX_TIMEOUT_MS", "300000")
	t.Setenv("SANDBOX_EXEC_MAX_OUTPUT_BYTES", "65536")
}
