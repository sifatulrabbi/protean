package entitlements

import (
	"context"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t *testing.T, rfc3339 string) *fakeClock {
	t.Helper()
	now, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse time %q: %v", rfc3339, err)
	}
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func (c *fakeClock) set(t *testing.T, rfc3339 string) {
	t.Helper()
	now, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse time %q: %v", rfc3339, err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = now
}

type usageKey struct{ orgID, month string }

type fakeTokenStore struct {
	mu    sync.Mutex
	usage map[usageKey][2]int64
	err   error
}

func newFakeTokenStore() *fakeTokenStore {
	return &fakeTokenStore{usage: map[usageKey][2]int64{}}
}

func (s *fakeTokenStore) AddUsage(_ context.Context, orgID, month string, in, out int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	k := usageKey{orgID, month}
	cur := s.usage[k]
	s.usage[k] = [2]int64{cur[0] + in, cur[1] + out}
	return nil
}

func (s *fakeTokenStore) UsageForMonth(_ context.Context, orgID, month string) (int64, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.err != nil {
		return 0, 0, s.err
	}
	v := s.usage[usageKey{orgID, month}]
	return v[0], v[1], nil
}

type fakeDisk struct {
	mu    sync.Mutex
	org   map[string]int64
	free  int64
	total int64
}

func newFakeDisk() *fakeDisk {
	return &fakeDisk{org: map[string]int64{}, free: 100, total: 100}
}

func (d *fakeDisk) setOrg(orgID string, bytes int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.org[orgID] = bytes
}

func (d *fakeDisk) setHost(free, total int64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.free, d.total = free, total
}

func (d *fakeDisk) OrgUsageBytes(_ context.Context, orgID string) (int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.org[orgID], nil
}

func (d *fakeDisk) HostDisk(context.Context) (int64, int64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.free, d.total, nil
}

func (d *fakeDisk) ListOrgs(context.Context) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	ids := make([]string, 0, len(d.org))
	for id := range d.org {
		ids = append(ids, id)
	}
	return ids, nil
}

type fakeCounts struct {
	members  map[string]int
	orgs     map[string]int
	projects map[string]int
}

func newFakeCounts() *fakeCounts {
	return &fakeCounts{members: map[string]int{}, orgs: map[string]int{}, projects: map[string]int{}}
}

func (c *fakeCounts) MemberCount(_ context.Context, orgID string) (int, error) {
	return c.members[orgID], nil
}

func (c *fakeCounts) OwnedOrgCount(_ context.Context, userID string) (int, error) {
	return c.orgs[userID], nil
}

func (c *fakeCounts) ProjectCount(_ context.Context, orgID string) (int, error) {
	return c.projects[orgID], nil
}

// recorder collects meter events in order.
type recorder struct {
	mu     sync.Mutex
	events []ports.MeterEvent
}

func (r *recorder) record(ev ports.MeterEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, ev)
}

func (r *recorder) snapshot() []ports.MeterEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ports.MeterEvent, len(r.events))
	copy(out, r.events)
	return out
}

func (r *recorder) thresholds(resource ports.MeterResource) []int {
	var out []int
	for _, ev := range r.snapshot() {
		if ev.Resource == resource {
			out = append(out, ev.ThresholdPct)
		}
	}
	return out
}

func (r *recorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = nil
}

type harness struct {
	engine *Engine
	clock  *fakeClock
	disk   *fakeDisk
	tokens *fakeTokenStore
	counts *fakeCounts
	events *recorder
	plan   Plan
}

// newHarness builds an engine on fakes with a zero watch interval, so every
// call re-measures and the tests never depend on cache timing.
func newHarness(t *testing.T) *harness {
	t.Helper()

	h := &harness{
		clock:  newFakeClock(t, "2026-03-15T10:00:00Z"),
		disk:   newFakeDisk(),
		tokens: newFakeTokenStore(),
		counts: newFakeCounts(),
		events: &recorder{},
		plan:   Free(),
	}
	h.engine = New(Deps{
		Plan:             h.plan,
		TokenUsage:       h.tokens,
		Disk:             h.disk,
		Counts:           h.counts,
		Clock:            h.clock,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		WatchInterval:    0,
		HostWatermarkPct: 80,
	})
	h.engine.Subscribe(h.events.record)
	return h
}
