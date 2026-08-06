package sandbox

import (
	"errors"
	"strings"
	"testing"
)

func TestResolvePath(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"plain file", "notes.md", "/workspace/notes.md", nil},
		{"nested", "src/pkg/main.go", "/workspace/src/pkg/main.go", nil},
		{"redundant segments", "./src/../src/main.go", "/workspace/src/main.go", nil},
		{"dot prefixed name", ".config/settings.json", "/workspace/.config/settings.json", nil},
		{"agents file is resolvable but read-only at the mount", "AGENTS.md", "/workspace/AGENTS.md", nil},

		{"empty", "", "", ErrPathEmpty},
		{"root", ".", "", ErrPathIsRoot},
		{"absolute", "/etc/passwd", "", ErrPathAbsolute},
		{"absolute inside workspace", "/workspace/notes.md", "", ErrPathAbsolute},
		{"parent", "..", "", ErrPathEscapes},
		{"traversal", "../../etc/passwd", "", ErrPathEscapes},
		{"traversal after descent", "src/../../etc/passwd", "", ErrPathEscapes},
		{"control dir", ".protean", "", ErrPathControl},
		{"inside control dir", ".protean/threads/01H.json", "", ErrPathControl},
		{"control dir nested", "src/.protean/x", "", ErrPathControl},
		{"control dir via traversal", "src/../.protean/x", "", ErrPathControl},
		{"control dir wrong case", ".PROTEAN/threads/x", "", ErrPathControl},
		{"control dir mixed case", ".Protean/x", "", ErrPathControl},
		{"nul byte", "src/ma\x00in.go", "", ErrPathInvalid},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePath(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ResolvePath(%q) err = %v, want %v", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvePath(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ResolvePath(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestResolveDir(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"empty is the root", "", WorkspaceRoot, nil},
		{"dot is the root", ".", WorkspaceRoot, nil},
		{"subdir", "src", "/workspace/src", nil},
		{"trailing slash", "src/", "/workspace/src", nil},
		{"absolute", "/tmp", "", ErrPathAbsolute},
		{"traversal", "../..", "", ErrPathEscapes},
		{"control dir", ".protean", "", ErrPathControl},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveDir(tc.in)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ResolveDir(%q) err = %v, want %v", tc.in, err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveDir(%q): %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("ResolveDir(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The rejection messages reach end users, so they must name the offending path.
func TestPathErrorsNamePath(t *testing.T) {
	_, err := ResolvePath("../../etc/passwd")
	if err == nil {
		t.Fatal("want an error")
	}
	if want := `"../../etc/passwd"`; !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %s", err, want)
	}
}
