package security

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWithinRoot(t *testing.T) {
	root := "/tmp/workspace"

	resolved, err := ResolveWithinRoot(root, "nested/file.txt")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resolved != "/tmp/workspace/nested/file.txt" {
		t.Fatalf("unexpected path: %s", resolved)
	}
}

func TestResolveWithinRootRejectsTraversal(t *testing.T) {
	root := "/tmp/workspace"

	if _, err := ResolveWithinRoot(root, "../outside.txt"); err == nil {
		t.Fatal("expected traversal error")
	}
}

func TestResolveSafePathRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.txt")
	if err := os.WriteFile(target, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write target: %v", err)
	}

	linkPath := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if _, err := ResolveSafePath(root, "link.txt"); err == nil {
		t.Fatal("expected symlink rejection")
	}
}
