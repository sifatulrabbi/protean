package security

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PathLocker coordinates file operations that would conflict on the same path tree.
type PathLocker struct {
	// mu guards all lock state below.
	mu sync.Mutex

	// cond wakes blocked callers whenever lock ownership changes.
	cond *sync.Cond

	// exactLocks counts in-flight operations on a specific file or directory path.
	exactLocks map[string]int
	// subtreeLocks counts in-flight operations that touch an entire directory subtree.
	subtreeLocks map[string]int
}

// NewPathLocker constructs an empty path-based lock coordinator.
func NewPathLocker() *PathLocker {
	pl := &PathLocker{
		exactLocks:   make(map[string]int),
		subtreeLocks: make(map[string]int),
	}
	pl.cond = sync.NewCond(&pl.mu)
	return pl
}

// LockExact blocks until the exact paths are free, then returns an idempotent unlock function.
func (pl *PathLocker) LockExact(paths ...string) (unlock func()) {
	keys := normalizeLockPaths(paths)
	if len(keys) == 0 {
		return func() {}
	}

	pl.mu.Lock()
	for !pl.canAcquireExact(keys) {
		pl.cond.Wait()
	}
	for _, key := range keys {
		pl.exactLocks[key]++
	}
	pl.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			pl.mu.Lock()
			for _, key := range keys {
				pl.exactLocks[key]--
				if pl.exactLocks[key] == 0 {
					delete(pl.exactLocks, key)
				}
			}
			pl.cond.Broadcast()
			pl.mu.Unlock()
		})
	}
}

// LockSubtree blocks until no overlapping exact or subtree locks exist.
func (pl *PathLocker) LockSubtree(paths ...string) (unlock func()) {
	keys := normalizeLockPaths(paths)
	if len(keys) == 0 {
		return func() {}
	}

	pl.mu.Lock()
	for !pl.canAcquireSubtree(keys) {
		pl.cond.Wait()
	}
	for _, key := range keys {
		pl.subtreeLocks[key]++
	}
	pl.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			pl.mu.Lock()
			for _, key := range keys {
				pl.subtreeLocks[key]--
				if pl.subtreeLocks[key] == 0 {
					delete(pl.subtreeLocks, key)
				}
			}
			pl.cond.Broadcast()
			pl.mu.Unlock()
		})
	}
}

// canAcquireExact reports whether every requested exact path is currently conflict-free.
func (pl *PathLocker) canAcquireExact(paths []string) bool {
	for _, path := range paths {
		if pl.exactLocks[path] > 0 {
			return false
		}
		for activeSubtree := range pl.subtreeLocks {
			if isSameOrDescendant(path, activeSubtree) {
				return false
			}
		}
	}
	return true
}

// canAcquireSubtree reports whether each subtree can be locked without overlap.
func (pl *PathLocker) canAcquireSubtree(paths []string) bool {
	for _, path := range paths {
		for activeExact := range pl.exactLocks {
			if isSameOrDescendant(activeExact, path) {
				return false
			}
		}
		for activeSubtree := range pl.subtreeLocks {
			if overlapsSubtree(path, activeSubtree) {
				return false
			}
		}
	}
	return true
}

// normalizeLockPaths canonicalizes, deduplicates, and sorts paths for deterministic locking.
func normalizeLockPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	uniq := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		uniq[canonicalPath(path)] = struct{}{}
	}

	// Sorting gives every caller the same acquisition order and avoids deadlocks.
	keys := make([]string, 0, len(uniq))
	for key := range uniq {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// canonicalPath converts a path to an absolute, cleaned form when possible.
func canonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	absolute, err := filepath.Abs(cleaned)
	if err != nil {
		return cleaned
	}
	return absolute
}

// overlapsSubtree reports whether two subtree locks would cover any common path.
func overlapsSubtree(a, b string) bool {
	return isSameOrDescendant(a, b) || isSameOrDescendant(b, a)
}

// isSameOrDescendant reports whether path is root itself or contained under it.
func isSameOrDescendant(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
