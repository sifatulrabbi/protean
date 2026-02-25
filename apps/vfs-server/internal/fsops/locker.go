package fsops

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// PathLocker coordinates conflicting exact-path and directory-subtree writes.
type PathLocker struct {
	mu sync.Mutex

	cond *sync.Cond

	exactLocks   map[string]int
	subtreeLocks map[string]int
}

func NewPathLocker() *PathLocker {
	pl := &PathLocker{
		exactLocks:   make(map[string]int),
		subtreeLocks: make(map[string]int),
	}
	pl.cond = sync.NewCond(&pl.mu)
	return pl
}

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

func normalizeLockPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}

	uniq := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		uniq[canonicalPath(path)] = struct{}{}
	}

	keys := make([]string, 0, len(uniq))
	for key := range uniq {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func canonicalPath(path string) string {
	cleaned := filepath.Clean(path)
	absolute, err := filepath.Abs(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(absolute)
}

func overlapsSubtree(a, b string) bool {
	return isSameOrDescendant(a, b) || isSameOrDescendant(b, a)
}

func isSameOrDescendant(path, root string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}
