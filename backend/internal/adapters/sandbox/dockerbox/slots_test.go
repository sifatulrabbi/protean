package dockerbox

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestSlotsCapAndHandoff(t *testing.T) {
	s := newSlots(2)
	ctx := context.Background()

	if err := s.acquire(ctx); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	if err := s.acquire(ctx); err != nil {
		t.Fatalf("second acquire: %v", err)
	}
	if got := s.inUse(); got != 2 {
		t.Fatalf("in use = %d, want 2", got)
	}

	// The third acquirer queues.
	acquired := make(chan struct{})
	go func() {
		if err := s.acquire(ctx); err != nil {
			t.Errorf("third acquire: %v", err)
			return
		}
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("third acquire went through while the cap was full")
	case <-time.After(20 * time.Millisecond):
	}

	s.release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("third acquire never got the released slot")
	}
	if got := s.inUse(); got != 2 {
		t.Errorf("in use = %d, want 2 after the hand-off", got)
	}
}

func TestSlotsAcquireRespectsContext(t *testing.T) {
	s := newSlots(1)
	if err := s.acquire(context.Background()); err != nil {
		t.Fatalf("acquire: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}

	// The abandoned waiter must not have leaked a slot.
	s.release()
	if got := s.inUse(); got != 0 {
		t.Errorf("in use = %d, want 0", got)
	}
}

// adopt counts containers that were already running at boot, even when that
// pushes the count over the cap; new acquirers then wait it out.
func TestSlotsAdoptMayExceedTheCap(t *testing.T) {
	s := newSlots(1)
	s.adopt()
	s.adopt()
	if got := s.inUse(); got != 2 {
		t.Fatalf("in use = %d, want 2", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded while over the cap", err)
	}

	s.release()
	s.release()
	if err := s.acquire(context.Background()); err != nil {
		t.Fatalf("acquire once back under the cap: %v", err)
	}
}

func TestSlotsConcurrentUseNeverExceedsTheCap(t *testing.T) {
	const limit = 3
	s := newSlots(limit)

	var mu sync.Mutex
	current, peak := 0, 0

	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := s.acquire(context.Background()); err != nil {
				t.Errorf("acquire: %v", err)
				return
			}
			mu.Lock()
			current++
			if current > peak {
				peak = current
			}
			mu.Unlock()

			time.Sleep(time.Millisecond)

			mu.Lock()
			current--
			mu.Unlock()
			s.release()
		}()
	}
	wg.Wait()

	if peak > limit {
		t.Errorf("peak concurrent holders = %d, want at most %d", peak, limit)
	}
	if got := s.inUse(); got != 0 {
		t.Errorf("in use = %d after everyone released, want 0", got)
	}
}
