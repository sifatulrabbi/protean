package fsops

import (
	"path/filepath"
	"testing"
)

func TestResolveWithinRoot(t *testing.T) {
	root := "/workspace/user1"

	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{"simple file", "file.txt", filepath.Join(root, "file.txt"), false},
		{"nested path", "a/b/c.txt", filepath.Join(root, "a/b/c.txt"), false},
		{"leading slash stripped", "/file.txt", filepath.Join(root, "file.txt"), false},
		{"dot path", ".", root, false},
		{"empty path", "", root, false},
		{"traversal blocked", "../escape", "", true},
		{"deep traversal blocked", "a/../../escape", "", true},
		{"double dot in middle", "a/../b", filepath.Join(root, "b"), false},
		{"whitespace trimmed", "  file.txt  ", filepath.Join(root, "file.txt"), false},
		{"absolute traversal", "/../../etc/passwd", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveWithinRoot(root, tt.input)
			if tt.wantError {
				if err == nil {
					t.Errorf("expected error for input %q, got %q", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error for input %q: %v", tt.input, err)
				return
			}
			if got != tt.want {
				t.Errorf("ResolveWithinRoot(%q, %q) = %q, want %q", root, tt.input, got, tt.want)
			}
		})
	}
}
