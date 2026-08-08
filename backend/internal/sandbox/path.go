// Package sandbox holds the adapter-independent half of the sandbox runtime:
// the workspace path guard every adapter must apply, and the registry that
// picks the one healthy adapter at boot.
package sandbox

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

const (
	// WorkspaceRoot is where the project directory is mounted inside every
	// sandbox. Nothing outside it is reachable.
	WorkspaceRoot = "/workspace"

	// ControlDirName is the per-project control directory. It holds threads,
	// memories, and skills, and is masked inside the sandbox — no path handed
	// to a sandbox may name it. The storage layout owns the name; this is an
	// alias so the mask and the guard can never drift apart.
	ControlDirName = layout.ControlDirName

	// AgentsFileName is mounted read-only inside the sandbox; its edits are
	// approval-gated and land host-side (D5).
	AgentsFileName = layout.AgentsFileName
)

// Path rejections. Match with errors.Is; the wrapped error names the offending
// path and is safe to show to an end user.
var (
	ErrPathEmpty    = errors.New("path is empty")
	ErrPathAbsolute = errors.New("path must be relative to the workspace root")
	ErrPathEscapes  = errors.New("path escapes the workspace root")
	ErrPathControl  = errors.New("path addresses the " + ControlDirName + " control directory")
	ErrPathInvalid  = errors.New("path contains an invalid character")
	ErrPathIsRoot   = errors.New("path is the workspace root, not a file")
)

// ResolvePath turns a workspace-relative path into its absolute path inside the
// sandbox. It rejects the empty path, absolute paths, traversal that leaves the
// workspace, and any path with a `.protean` segment.
//
// The control-directory check is case-insensitive on purpose: the workspace is
// bind-mounted from the host, and on a case-insensitive host filesystem
// (APFS by default) `.PROTEAN` would resolve to the same directory and slip
// past the mask.
func ResolvePath(rel string) (string, error) {
	if rel == "" {
		return "", ErrPathEmpty
	}
	clean, err := cleanRel(rel)
	if err != nil {
		return "", err
	}
	if clean == "." {
		return "", fmt.Errorf("%q: %w", rel, ErrPathIsRoot)
	}
	return path.Join(WorkspaceRoot, clean), nil
}

// ResolveDir is ResolvePath for a directory argument, where the empty path is
// legal and means the workspace root.
func ResolveDir(rel string) (string, error) {
	clean, err := cleanRel(rel)
	if err != nil {
		return "", err
	}
	if clean == "." {
		return WorkspaceRoot, nil
	}
	return path.Join(WorkspaceRoot, clean), nil
}

// cleanRel validates rel and returns it cleaned. "" and "." both clean to ".",
// which callers interpret as the workspace root.
func cleanRel(rel string) (string, error) {
	if strings.ContainsRune(rel, 0) {
		return "", fmt.Errorf("%q: %w", rel, ErrPathInvalid)
	}
	if strings.HasPrefix(rel, "/") {
		return "", fmt.Errorf("%q: %w", rel, ErrPathAbsolute)
	}

	clean := path.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("%q: %w", rel, ErrPathEscapes)
	}
	for _, seg := range strings.Split(clean, "/") {
		if strings.EqualFold(seg, ControlDirName) {
			return "", fmt.Errorf("%q: %w", rel, ErrPathControl)
		}
	}
	return clean, nil
}
