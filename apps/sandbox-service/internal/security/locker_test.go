package security

import (
	"sync"
	"testing"
	"time"
)

func TestPathLockerExactSamePathSerializes(t *testing.T) {
	pl := NewPathLocker()
	path := "/x/a.txt"

	firstUnlock := pl.LockExact(path)
	acquiredSecond := make(chan struct{})
	go func() {
		unlock := pl.LockExact(path)
		unlock()
		close(acquiredSecond)
	}()

	assertBlocked(t, acquiredSecond)
	firstUnlock()
	assertAcquired(t, acquiredSecond)
}

func TestPathLockerEmptyAcquireIsNoOp(t *testing.T) {
	pl := NewPathLocker()

	unlockExact := pl.LockExact()
	unlockSubtree := pl.LockSubtree()

	unlockExact()
	unlockSubtree()

	if len(pl.exactLocks) != 0 {
		t.Fatalf("expected no exact locks, got %d", len(pl.exactLocks))
	}
	if len(pl.subtreeLocks) != 0 {
		t.Fatalf("expected no subtree locks, got %d", len(pl.subtreeLocks))
	}
}

func TestPathLockerExactDifferentPathsAreConcurrent(t *testing.T) {
	pl := NewPathLocker()

	firstUnlock := pl.LockExact("/x/a.txt")
	acquiredSecond := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/b.txt")
		unlock()
		close(acquiredSecond)
	}()

	assertAcquired(t, acquiredSecond)
	firstUnlock()
}

func TestPathLockerCanonicalizationMakesEquivalentExactPathsConflict(t *testing.T) {
	pl := NewPathLocker()

	firstUnlock := pl.LockExact("/x/dir/../a.txt")
	acquiredSecond := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/a.txt")
		unlock()
		close(acquiredSecond)
	}()

	assertBlocked(t, acquiredSecond)
	firstUnlock()
	assertAcquired(t, acquiredSecond)
}

func TestPathLockerSubtreeBlocksExactDescendant(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir")
	acquiredExact := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/dir/a.txt")
		unlock()
		close(acquiredExact)
	}()

	assertBlocked(t, acquiredExact)
	unlockSubtree()
	assertAcquired(t, acquiredExact)
}

func TestPathLockerSubtreeBlocksExactSamePath(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir")
	acquiredExact := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/dir")
		unlock()
		close(acquiredExact)
	}()

	assertBlocked(t, acquiredExact)
	unlockSubtree()
	assertAcquired(t, acquiredExact)
}

func TestPathLockerSubtreeDescendantDoesNotBlockExactAncestor(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir/sub")
	acquiredExact := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/dir")
		unlock()
		close(acquiredExact)
	}()

	assertAcquired(t, acquiredExact)
	unlockSubtree()
}

func TestPathLockerExactAncestorDoesNotBlockSubtreeDescendant(t *testing.T) {
	pl := NewPathLocker()

	unlockExact := pl.LockExact("/x/dir")
	acquiredSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir/sub")
		unlock()
		close(acquiredSubtree)
	}()

	assertAcquired(t, acquiredSubtree)
	unlockExact()
}

func TestPathLockerDisjointSubtreeAndExactAreConcurrent(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir1")
	acquiredExact := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/dir2/a.txt")
		unlock()
		close(acquiredExact)
	}()

	assertAcquired(t, acquiredExact)
	unlockSubtree()
}

func TestPathLockerDisjointSubtreesAreConcurrent(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir1")
	acquiredSecondSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir2")
		unlock()
		close(acquiredSecondSubtree)
	}()

	assertAcquired(t, acquiredSecondSubtree)
	unlockSubtree()
}

func TestPathLockerOverlappingSubtreesConflict(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir")
	acquiredOverlappingSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir/sub")
		unlock()
		close(acquiredOverlappingSubtree)
	}()

	assertBlocked(t, acquiredOverlappingSubtree)
	unlockSubtree()
	assertAcquired(t, acquiredOverlappingSubtree)
}

func TestPathLockerAncestorSubtreeRequestBlockedByDescendantSubtree(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir/sub")
	acquiredAncestorSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir")
		unlock()
		close(acquiredAncestorSubtree)
	}()

	assertBlocked(t, acquiredAncestorSubtree)
	unlockSubtree()
	assertAcquired(t, acquiredAncestorSubtree)
}

func TestPathLockerSubtreeRequestBlockedByActiveExactDescendant(t *testing.T) {
	pl := NewPathLocker()

	unlockExact := pl.LockExact("/x/dir/a.txt")
	acquiredSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir")
		unlock()
		close(acquiredSubtree)
	}()

	assertBlocked(t, acquiredSubtree)
	unlockExact()
	assertAcquired(t, acquiredSubtree)
}

func TestPathLockerSubtreeDisjointFromActiveExactIsConcurrent(t *testing.T) {
	pl := NewPathLocker()

	unlockExact := pl.LockExact("/x/dir1/a.txt")
	acquiredSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir2")
		unlock()
		close(acquiredSubtree)
	}()

	assertAcquired(t, acquiredSubtree)
	unlockExact()
}

func TestPathLockerRootSubtreeBlocksEverythingBelowRoot(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/")
	acquiredExact := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/a.txt")
		unlock()
		close(acquiredExact)
	}()

	assertBlocked(t, acquiredExact)
	unlockSubtree()
	assertAcquired(t, acquiredExact)
}

func TestPathLockerRootSubtreeBlocksNestedSubtrees(t *testing.T) {
	pl := NewPathLocker()

	unlockRoot := pl.LockSubtree("/")
	acquiredSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir")
		unlock()
		close(acquiredSubtree)
	}()

	assertBlocked(t, acquiredSubtree)
	unlockRoot()
	assertAcquired(t, acquiredSubtree)
}

func TestPathLockerSubtreeCanonicalizationMakesEquivalentPathsConflict(t *testing.T) {
	pl := NewPathLocker()

	unlockSubtree := pl.LockSubtree("/x/dir/../dir")
	acquiredSubtree := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/dir/sub")
		unlock()
		close(acquiredSubtree)
	}()

	assertBlocked(t, acquiredSubtree)
	unlockSubtree()
	assertAcquired(t, acquiredSubtree)
}

func TestPathLockerExactMultiPathAcquireIsAtomic(t *testing.T) {
	pl := NewPathLocker()

	unlockA := pl.LockExact("/x/a.txt")
	acquiredAB := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/a.txt", "/x/b.txt")
		unlock()
		close(acquiredAB)
	}()

	assertBlocked(t, acquiredAB)

	acquiredB := make(chan struct{})
	go func() {
		unlock := pl.LockExact("/x/b.txt")
		unlock()
		close(acquiredB)
	}()
	assertAcquired(t, acquiredB)

	unlockA()
	assertAcquired(t, acquiredAB)
}

func TestPathLockerAtomicDualExactLockAvoidsInverseOrderDeadlock(t *testing.T) {
	pl := NewPathLocker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		start := make(chan struct{})
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			unlock := pl.LockExact("/x/a.txt", "/x/b.txt")
			time.Sleep(time.Millisecond)
			unlock()
		}()

		go func() {
			defer wg.Done()
			<-start
			unlock := pl.LockExact("/x/b.txt", "/x/a.txt")
			time.Sleep(time.Millisecond)
			unlock()
		}()

		close(start)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for inverse exact lock attempts to complete")
	}
}

func TestPathLockerDuplicatePathsAreDeduplicated(t *testing.T) {
	pl := NewPathLocker()

	unlock := pl.LockExact("/x/a.txt", "/x/a.txt", "/x/dir/../a.txt")
	if len(pl.exactLocks) != 1 {
		t.Fatalf("expected exactly one exact lock entry after deduplication, got %d", len(pl.exactLocks))
	}

	unlock()
	if len(pl.exactLocks) != 0 {
		t.Fatalf("expected exact lock entry cleanup after unlock, got %d", len(pl.exactLocks))
	}
}

func TestPathLockerDuplicateSubtreePathsAreDeduplicated(t *testing.T) {
	pl := NewPathLocker()

	unlock := pl.LockSubtree("/x/dir", "/x/dir", "/x/a/../dir")
	if len(pl.subtreeLocks) != 1 {
		t.Fatalf("expected exactly one subtree lock entry after deduplication, got %d", len(pl.subtreeLocks))
	}

	unlock()
	if len(pl.subtreeLocks) != 0 {
		t.Fatalf("expected subtree lock entry cleanup after unlock, got %d", len(pl.subtreeLocks))
	}
}

func TestPathLockerUnlockIsIdempotent(t *testing.T) {
	pl := NewPathLocker()

	unlock := pl.LockSubtree("/x/dir")
	unlock()
	unlock()

	if len(pl.subtreeLocks) != 0 {
		t.Fatalf("expected subtree lock entry cleanup after repeated unlock, got %d", len(pl.subtreeLocks))
	}
}

func TestPathLockerNoOpUnlockIsIdempotent(t *testing.T) {
	pl := NewPathLocker()

	unlock := pl.LockExact()
	unlock()
	unlock()

	if len(pl.exactLocks) != 0 {
		t.Fatalf("expected no exact lock entries, got %d", len(pl.exactLocks))
	}
}

func TestPathLockerAtomicDualPathLockAvoidsInverseRenameDeadlock(t *testing.T) {
	pl := NewPathLocker()
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		start := make(chan struct{})
		wg.Add(2)

		go func() {
			defer wg.Done()
			<-start
			unlock := pl.LockSubtree("/x/src", "/x/dst")
			time.Sleep(time.Millisecond)
			unlock()
		}()

		go func() {
			defer wg.Done()
			<-start
			unlock := pl.LockSubtree("/x/dst", "/x/src")
			time.Sleep(time.Millisecond)
			unlock()
		}()

		close(start)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for inverse rename lock attempts to complete")
	}
}

func TestPathLockerRefCountCleanup(t *testing.T) {
	pl := NewPathLocker()

	unlockExact := pl.LockExact("/x/a.txt")
	unlockSubtree := pl.LockSubtree("/x/dir")

	if len(pl.exactLocks) != 1 {
		t.Fatalf("expected 1 exact lock entry while held, got %d", len(pl.exactLocks))
	}
	if len(pl.subtreeLocks) != 1 {
		t.Fatalf("expected 1 subtree lock entry while held, got %d", len(pl.subtreeLocks))
	}

	unlockExact()
	unlockSubtree()

	if len(pl.exactLocks) != 0 {
		t.Fatalf("expected exact lock entries cleaned up, got %d", len(pl.exactLocks))
	}
	if len(pl.subtreeLocks) != 0 {
		t.Fatalf("expected subtree lock entries cleaned up, got %d", len(pl.subtreeLocks))
	}
}

func TestPathLockerSubtreeMultiPathAcquireIsAtomic(t *testing.T) {
	pl := NewPathLocker()

	unlockHeld := pl.LockSubtree("/x/a")
	acquiredAB := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/a", "/x/b")
		unlock()
		close(acquiredAB)
	}()

	assertBlocked(t, acquiredAB)

	acquiredB := make(chan struct{})
	go func() {
		unlock := pl.LockSubtree("/x/b")
		unlock()
		close(acquiredB)
	}()
	assertAcquired(t, acquiredB)

	unlockHeld()
	assertAcquired(t, acquiredAB)
}

func TestPathLockerRelativeAndAbsoluteEquivalentExactPathsConflict(t *testing.T) {
	pl := NewPathLocker()

	unlock := pl.LockExact("./tmp/../tmp/a.txt")
	acquired := make(chan struct{})
	go func() {
		unlockSecond := pl.LockExact("tmp/a.txt")
		unlockSecond()
		close(acquired)
	}()

	assertBlocked(t, acquired)
	unlock()
	assertAcquired(t, acquired)
}

func assertBlocked(t *testing.T, acquired <-chan struct{}) {
	t.Helper()

	select {
	case <-acquired:
		t.Fatal("lock unexpectedly acquired while conflicting lock was held")
	case <-time.After(100 * time.Millisecond):
	}
}

func assertAcquired(t *testing.T, acquired <-chan struct{}) {
	t.Helper()

	select {
	case <-acquired:
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for lock acquisition")
	}
}
