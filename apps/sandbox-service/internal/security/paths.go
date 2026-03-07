package security

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrSymlinkDetected is returned when a path component is a symlink.
var ErrSymlinkDetected = errors.New("symlink detected in path")

// ResolveWithinRoot cleans a user-supplied path and ensures it stays under root.
func ResolveWithinRoot(root, inputPath string) (string, error) {
	sanitized := strings.TrimSpace(inputPath)
	relativeInput := strings.TrimPrefix(sanitized, "/")

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

// ResolveSafePath rejects traversal and any existing symlink in the resolved path.
func ResolveSafePath(root, inputPath string) (string, error) {
	resolved, err := ResolveWithinRoot(root, inputPath)
	if err != nil {
		return "", err
	}

	relativePath, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", fmt.Errorf("path %q escapes workspace root", inputPath)
	}

	current := filepath.Clean(root)
	if relativePath == "." {
		if err := rejectSymlink(current, inputPath); err != nil {
			return "", err
		}
		return resolved, nil
	}

	// Walk each existing path component so a nested symlink cannot redirect later filesystem calls.
	for _, part := range strings.Split(relativePath, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := rejectSymlink(current, inputPath); err != nil {
			return "", err
		}
	}

	return resolved, nil
}

// rejectSymlink rejects candidatePath when it already exists as a symlink.
func rejectSymlink(candidatePath, inputPath string) error {
	info, err := os.Lstat(candidatePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("path %q resolves through a symlink: %w", inputPath, ErrSymlinkDetected)
	}

	return nil
}
