//go:build integration

package sandbox

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

// TestSDKSmokeTest exercises the real Docker SDK client against a running daemon.
// Run with: go test ./sandbox/ -tags=integration -run TestSDKSmoke -v
func TestSDKSmokeTest(t *testing.T) {
	// Create a temp workspace directory
	tmpDir, err := os.MkdirTemp("", "sandbox-sdk-smoke-*")
	if err != nil {
		t.Fatalf("create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runtime, err := NewDockerRuntime("root", "/workspace")
	if err != nil {
		t.Fatalf("NewDockerRuntime: %v", err)
	}

	meta := SessionMetadata{
		SessionID:          "sdk-smoke-test",
		WorkspaceFullPath:  tmpDir,
		WorkspaceMountPath: "/workspace",
		ContainerName:      "protean-sdk-smoke-test",
		Image:              "debian:bookworm-slim",
	}
	ctx := context.Background()

	// Clean up any leftover container from previous runs
	_ = runtime.RemoveContainer(ctx, meta)

	// 1. Inspect should return ErrContainerNotFound
	t.Log("Step 1: Inspect non-existent container")
	_, err = runtime.InspectContainer(ctx, meta)
	if err != ErrContainerNotFound {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
	t.Log("  -> correctly got ErrContainerNotFound")

	// 2. EnsureContainer should create and start it
	t.Log("Step 2: EnsureContainer (create + start)")
	state, err := runtime.EnsureContainer(ctx, meta)
	if err != nil {
		t.Fatalf("EnsureContainer: %v", err)
	}
	if !state.Running {
		t.Fatalf("expected running, got state=%+v", state)
	}
	t.Logf("  -> container running: id=%s image=%s state=%s", state.ID[:12], state.Image, state.State)

	// 3. Inspect should now return running state
	t.Log("Step 3: Inspect running container")
	state, err = runtime.InspectContainer(ctx, meta)
	if err != nil {
		t.Fatalf("InspectContainer: %v", err)
	}
	if !state.Present || !state.Running {
		t.Fatalf("expected present+running, got %+v", state)
	}
	t.Log("  -> present=true running=true")

	// 4. EnsureContainer again should be idempotent (container already running)
	t.Log("Step 4: EnsureContainer (idempotent, already running)")
	state2, err := runtime.EnsureContainer(ctx, meta)
	if err != nil {
		t.Fatalf("EnsureContainer (idempotent): %v", err)
	}
	if state2.ID != state.ID {
		t.Fatalf("expected same container id, got %s vs %s", state.ID, state2.ID)
	}
	t.Log("  -> same container ID, idempotent OK")

	// 5. Exec: simple echo
	t.Log("Step 5: Exec echo command")
	result, err := runtime.Exec(ctx, meta, "/workspace", "echo 'hello from SDK'", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec echo: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%q", result.ExitCode, result.Stderr)
	}
	t.Logf("  -> stdout=%q exit=%d", result.Stdout, result.ExitCode)

	// 6. Exec: non-zero exit code
	t.Log("Step 6: Exec with non-zero exit")
	result, err = runtime.Exec(ctx, meta, "/workspace", "exit 42", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec exit 42: %v", err)
	}
	if result.ExitCode != 42 {
		t.Fatalf("expected exit 42, got %d", result.ExitCode)
	}
	t.Logf("  -> exit=%d (correct)", result.ExitCode)

	// 7. Exec: stderr output
	t.Log("Step 7: Exec with stderr")
	result, err = runtime.Exec(ctx, meta, "/workspace", "echo err >&2", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec stderr: %v", err)
	}
	t.Logf("  -> stdout=%q stderr=%q", result.Stdout, result.Stderr)

	// 8. Exec: working directory
	t.Log("Step 8: Exec with custom working directory")
	// First create a subdirectory
	_, err = runtime.Exec(ctx, meta, "/workspace", "mkdir -p /workspace/subdir", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec mkdir: %v", err)
	}
	result, err = runtime.Exec(ctx, meta, "/workspace/subdir", "pwd", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec pwd: %v", err)
	}
	t.Logf("  -> pwd=%q", result.Stdout)

	// 9. Exec: timeout
	t.Log("Step 9: Exec with timeout")
	result, err = runtime.Exec(ctx, meta, "/workspace", "sleep 30", 100*time.Millisecond, 4096)
	if err != nil {
		t.Fatalf("Exec timeout: %v", err)
	}
	if !result.TimedOut {
		t.Fatalf("expected timeout, got %+v", result)
	}
	t.Logf("  -> timedOut=%v exitCode=%d (correct)", result.TimedOut, result.ExitCode)

	// 10. Exec: signal detection (SIGTERM = 128+15=143)
	t.Log("Step 10: Exec with signal (SIGTERM)")
	result, err = runtime.Exec(ctx, meta, "/workspace", "kill -TERM $$", 5*time.Second, 4096)
	if err != nil {
		t.Fatalf("Exec signal: %v", err)
	}
	t.Logf("  -> exitCode=%d signalCode=%q", result.ExitCode, result.SignalCode)

	// 11. Remove container
	t.Log("Step 11: RemoveContainer")
	if err := runtime.RemoveContainer(ctx, meta); err != nil {
		t.Fatalf("RemoveContainer: %v", err)
	}
	t.Log("  -> removed OK")

	// 12. Remove again should be idempotent
	t.Log("Step 12: RemoveContainer (idempotent)")
	if err := runtime.RemoveContainer(ctx, meta); err != nil {
		t.Fatalf("RemoveContainer (idempotent): %v", err)
	}
	t.Log("  -> idempotent remove OK")

	// 13. Verify container is gone
	t.Log("Step 13: Verify container removed")
	_, err = runtime.InspectContainer(ctx, meta)
	if err != ErrContainerNotFound {
		t.Fatalf("expected ErrContainerNotFound, got %v", err)
	}
	t.Log("  -> correctly got ErrContainerNotFound")

	fmt.Println("\n=== ALL SDK SMOKE TESTS PASSED ===")
}
