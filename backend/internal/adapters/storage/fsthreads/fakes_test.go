package fsthreads

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

// fakeEntitlements records every disk preflight and can be flipped to reject.
type fakeEntitlements struct {
	mu     sync.Mutex
	err    error
	deltas []int64
}

var _ ports.Entitlements = (*fakeEntitlements)(nil)

func newFakeEntitlements() *fakeEntitlements { return &fakeEntitlements{} }

func (f *fakeEntitlements) reject(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeEntitlements) recorded() []int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int64(nil), f.deltas...)
}

func (f *fakeEntitlements) CheckDiskWrite(_ context.Context, _ string, deltaBytes int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deltas = append(f.deltas, deltaBytes)
	return f.err
}

func (f *fakeEntitlements) CheckLLMInvocation(context.Context, string) error { return nil }

func (f *fakeEntitlements) RecordTokenUsage(context.Context, string, int64, int64) error { return nil }

func (f *fakeEntitlements) CanAddMember(context.Context, string) error { return nil }

func (f *fakeEntitlements) CanCreateOrg(context.Context, string) error { return nil }

func (f *fakeEntitlements) CanCreateProject(context.Context, string) error { return nil }

func (f *fakeEntitlements) Meters(context.Context, string) (ports.Meters, error) {
	return ports.Meters{}, nil
}

func (f *fakeEntitlements) Subscribe(func(ports.MeterEvent)) {}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
