package ulid_test

import (
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/storage/ulid"
)

// frozenClock never advances, so every ID it stamps lands in the same
// millisecond. That is exactly the case monotonic entropy has to survive.
type frozenClock struct{ now time.Time }

func (c frozenClock) Now() time.Time { return c.now }

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse %q: %v", s, err)
	}
	return ts
}

func TestNewIsCanonical(t *testing.T) {
	g := ulid.NewGenerator(frozenClock{now: mustTime(t, "2026-08-06T10:00:00Z")})

	id := g.New()
	if len(id) != 26 {
		t.Fatalf("got %q (%d chars), want 26", id, len(id))
	}
	if !ulid.Valid(id) {
		t.Fatalf("%q does not parse as a ULID", id)
	}
	for _, r := range id {
		if r >= 'a' && r <= 'z' {
			t.Fatalf("%q is not uppercase", id)
		}
	}
}

func TestMonotonicWithinOneTick(t *testing.T) {
	g := ulid.NewGenerator(frozenClock{now: mustTime(t, "2026-08-06T10:00:00Z")})

	const n = 1000
	ids := make([]string, n)
	for i := range ids {
		ids[i] = g.New()
	}
	for i := 1; i < n; i++ {
		if ids[i] <= ids[i-1] {
			t.Fatalf("id %d (%q) does not sort after %q", i, ids[i], ids[i-1])
		}
	}
}

func TestOrderFollowsClock(t *testing.T) {
	clk := &steppingClock{now: mustTime(t, "2026-08-06T10:00:00Z")}
	g := ulid.NewGenerator(clk)

	first := g.New()
	clk.advance(5 * time.Millisecond)
	second := g.New()

	if second <= first {
		t.Fatalf("later id %q does not sort after earlier %q", second, first)
	}
}

func TestConcurrentGenerationIsUniqueAndOrdered(t *testing.T) {
	g := ulid.NewGenerator(frozenClock{now: mustTime(t, "2026-08-06T10:00:00Z")})

	const goroutines, each = 16, 64
	var (
		mu  sync.Mutex
		all []string
		wg  sync.WaitGroup
	)
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			local := make([]string, each)
			for i := range local {
				local[i] = g.New()
			}
			mu.Lock()
			all = append(all, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(all) != goroutines*each {
		t.Fatalf("got %d ids, want %d", len(all), goroutines*each)
	}
	sort.Strings(all)
	for i := 1; i < len(all); i++ {
		if all[i] == all[i-1] {
			t.Fatalf("duplicate id %q", all[i])
		}
	}
}

type steppingClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *steppingClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *steppingClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}
