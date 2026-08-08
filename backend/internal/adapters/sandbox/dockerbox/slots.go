package dockerbox

import (
	"context"
	"sync"
)

// slots is the global cap on concurrently running sandboxes. It is a counting
// semaphore with FIFO hand-off, plus an adopt operation the boot-time scan uses
// to account for containers that were already running before this process
// started. Adoption may push the count above the cap; new acquirers then wait
// until it drops back under, rather than the runtime killing work it did not
// start.
type slots struct {
	mu      sync.Mutex
	limit   int
	used    int
	waiters []chan struct{}
}

func newSlots(limit int) *slots {
	if limit < 1 {
		limit = 1
	}
	return &slots{limit: limit}
}

// acquire takes a slot, blocking until one frees or ctx ends.
func (s *slots) acquire(ctx context.Context) error {
	s.mu.Lock()
	if s.used < s.limit {
		s.used++
		s.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	s.waiters = append(s.waiters, ch)
	s.mu.Unlock()

	select {
	case <-ch:
		// A releaser handed its slot straight to us; used is unchanged.
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		for i, w := range s.waiters {
			if w == ch {
				s.waiters = append(s.waiters[:i], s.waiters[i+1:]...)
				s.mu.Unlock()
				return ctx.Err()
			}
		}
		s.mu.Unlock()
		// We lost the race: a releaser already handed us the slot, so give it
		// back instead of leaking it.
		s.release()
		return ctx.Err()
	}
}

// adopt counts a slot that is already in use without waiting for room.
func (s *slots) adopt() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.used++
}

// release gives a slot back, handing it to the longest-waiting acquirer if any.
func (s *slots) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used > s.limit {
		s.used--
		return
	}
	if s.used > 0 {
		s.used--
	}
	if len(s.waiters) > 0 && s.used < s.limit {
		s.used++
		ch := s.waiters[0]
		s.waiters = s.waiters[1:]
		close(ch)
	}
}

// inUse is the current count, for logging and tests.
func (s *slots) inUse() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.used
}
