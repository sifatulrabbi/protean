package fsops

import (
	"fmt"
	"path/filepath"
	"strings"
)

// ResolveWithinRoot resolves inputPath relative to root and ensures the result
// does not escape the root directory. This is the Go port of resolveWithinRoot
// from packages/vfs/src/path-utils.ts.
func ResolveWithinRoot(root, inputPath string) (string, error) {
	sanitized := strings.TrimSpace(inputPath)
	// Strip leading slash to make it relative
	relativeInput := sanitized
	relativeInput = strings.TrimPrefix(relativeInput, "/")

	candidate := filepath.Join(root, relativeInput)
	candidate = filepath.Clean(candidate)

	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return "", fmt.Errorf("path %q escapes workspace root", inputPath)
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path %q escapes workspace root", inputPath)
	}

	return candidate, nil
}
