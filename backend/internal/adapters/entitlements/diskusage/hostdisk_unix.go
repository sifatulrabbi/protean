//go:build unix

package diskusage

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// hostDisk statfs's path, walking up to the nearest existing ancestor when the
// data dir has not been created yet.
func hostDisk(path string) (freeBytes, totalBytes int64, err error) {
	target, err := existingAncestor(path)
	if err != nil {
		return 0, 0, err
	}

	var st unix.Statfs_t
	if err := unix.Statfs(target, &st); err != nil {
		return 0, 0, fmt.Errorf("statfs %q: %w", target, err)
	}

	// Bsize is int64 on linux and uint32 on darwin; Bavail and Blocks are
	// uint64 on both. The conversions keep one implementation for each.
	blockSize := int64(st.Bsize)
	return int64(st.Bavail) * blockSize, int64(st.Blocks) * blockSize, nil
}

func existingAncestor(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %q: %w", path, err)
	}

	for {
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return abs, nil
		}
		abs = parent
	}
}
