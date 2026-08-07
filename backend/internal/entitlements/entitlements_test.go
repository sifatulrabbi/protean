package entitlements

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

func TestCheckDiskWrite(t *testing.T) {
	plan := Free()

	tests := []struct {
		name      string
		orgBytes  int64
		hostFree  int64
		hostTotal int64
		delta     int64
		wantErr   error
	}{
		{name: "empty org", delta: 1024, hostFree: 100, hostTotal: 100},
		{name: "fits exactly", orgBytes: plan.DiskLimitBytes - 10, delta: 10, hostFree: 100, hostTotal: 100},
		{name: "delta crosses cap", orgBytes: plan.DiskLimitBytes - 10, delta: 11, hostFree: 100, hostTotal: 100, wantErr: ports.ErrDiskQuotaExceeded},
		{name: "org at cap is read-only", orgBytes: plan.DiskLimitBytes, delta: 1, hostFree: 100, hostTotal: 100, wantErr: ports.ErrOrgReadOnly},
		{name: "delete allowed while read-only", orgBytes: plan.DiskLimitBytes, delta: -1, hostFree: 100, hostTotal: 100},
		{name: "host out of space", delta: 1, hostFree: 10, hostTotal: 100, wantErr: ports.ErrOutOfSpace},
		{name: "host at watermark boundary", delta: 1, hostFree: 20, hostTotal: 100, wantErr: ports.ErrOutOfSpace},
		{name: "host just under watermark", delta: 1, hostFree: 21, hostTotal: 100},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			h.disk.setOrg("org-1", tc.orgBytes)
			h.disk.setHost(tc.hostFree, tc.hostTotal)

			err := h.engine.CheckDiskWrite(context.Background(), "org-1", tc.delta)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckDiskWrite = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestOrgReadOnlySwitchFlipsAndRecovers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.disk.setOrg("org-1", h.plan.DiskLimitBytes/2)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", 1); err != nil {
		t.Fatalf("write below cap: %v", err)
	}

	h.disk.setOrg("org-1", h.plan.DiskLimitBytes)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", 1); !errors.Is(err, ports.ErrOrgReadOnly) {
		t.Fatalf("write at cap = %v, want ErrOrgReadOnly", err)
	}
	meters, err := h.engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if !meters.OrgReadOnly {
		t.Fatal("Meters.OrgReadOnly = false, want true while at cap")
	}

	h.disk.setOrg("org-1", h.plan.DiskLimitBytes/2)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", 1); err != nil {
		t.Fatalf("write after freeing space: %v", err)
	}
	if meters, err = h.engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.OrgReadOnly {
		t.Fatal("Meters.OrgReadOnly = true, want false after recovery")
	}
}

func TestOutOfSpaceSwitchFlipsAndRecovers(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.disk.setOrg("org-1", 0)

	h.disk.setHost(5, 100)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", 1); !errors.Is(err, ports.ErrOutOfSpace) {
		t.Fatalf("write on full host = %v, want ErrOutOfSpace", err)
	}

	h.disk.setHost(50, 100)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", 1); err != nil {
		t.Fatalf("write after host recovered: %v", err)
	}
}

func TestNoCreditsSwitchAndMonthlyReset(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	if err := h.engine.CheckLLMInvocation(ctx, "org-1"); err != nil {
		t.Fatalf("first invocation: %v", err)
	}

	if err := h.engine.RecordTokenUsage(ctx, "org-1", h.plan.MonthlyTokenLimit-1, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}
	if err := h.engine.CheckLLMInvocation(ctx, "org-1"); err != nil {
		t.Fatalf("invocation with 1 token left: %v", err)
	}

	if err := h.engine.RecordTokenUsage(ctx, "org-1", 0, 1); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}
	if err := h.engine.CheckLLMInvocation(ctx, "org-1"); !errors.Is(err, ports.ErrNoCredits) {
		t.Fatalf("invocation at limit = %v, want ErrNoCredits", err)
	}

	meters, err := h.engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if !meters.NoCredits || meters.TokensUsed != h.plan.MonthlyTokenLimit || meters.Month != "2026-03" {
		t.Fatalf("Meters = %+v, want no credits at limit in 2026-03", meters)
	}

	h.clock.set(t, "2026-04-01T00:00:00Z")
	if err := h.engine.CheckLLMInvocation(ctx, "org-1"); err != nil {
		t.Fatalf("invocation after monthly reset: %v", err)
	}
	if meters, err = h.engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.NoCredits || meters.TokensUsed != 0 || meters.Month != "2026-04" {
		t.Fatalf("Meters after reset = %+v, want zero usage in 2026-04", meters)
	}
}

func TestConcurrentLLMReservationsNearCapDoNotOvershoot(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	remaining := int64(10)
	if err := h.engine.RecordTokenUsage(ctx, "org-1", h.plan.MonthlyTokenLimit-remaining, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}

	const invocations = 32
	start := make(chan struct{})
	results := make(chan ports.LLMReservation, invocations)
	errs := make(chan error, invocations)
	var wg sync.WaitGroup
	for range invocations {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reservation, err := h.engine.ReserveLLMInvocation(ctx, "org-1")
			if err != nil {
				errs <- err
				return
			}
			results <- reservation
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	var reservations []ports.LLMReservation
	for reservation := range results {
		reservations = append(reservations, reservation)
	}
	if len(reservations) != 1 {
		t.Fatalf("successful reservations = %d, want 1", len(reservations))
	}
	for err := range errs {
		if !errors.Is(err, ports.ErrNoCredits) {
			t.Errorf("reservation error = %v, want ErrNoCredits", err)
		}
	}
	if err := reservations[0].Settle(ctx, remaining, 0); err != nil {
		t.Fatalf("Settle: %v", err)
	}

	meters, err := h.engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.TokensUsed != h.plan.MonthlyTokenLimit {
		t.Fatalf("TokensUsed = %d, want %d", meters.TokensUsed, h.plan.MonthlyTokenLimit)
	}
}

func TestLLMReservationsAllowConcurrencyWhenBudgetIsPlentiful(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	const concurrent = 5
	start := make(chan struct{})
	results := make(chan ports.LLMReservation, concurrent)
	errs := make(chan error, concurrent)
	var wg sync.WaitGroup
	for i := range concurrent {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			reservation, err := h.engine.ReserveLLMInvocation(ctx, "org-1")
			if err != nil {
				errs <- fmt.Errorf("reservation %d: %w", i+1, err)
				return
			}
			results <- reservation
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)

	for err := range errs {
		t.Error(err)
	}
	reservations := make([]ports.LLMReservation, 0, concurrent)
	for reservation := range results {
		reservations = append(reservations, reservation)
	}
	if len(reservations) != concurrent {
		t.Fatalf("successful reservations = %d, want %d", len(reservations), concurrent)
	}
	if err := h.engine.CheckLLMInvocation(ctx, "org-1"); err != nil {
		t.Fatalf("CheckLLMInvocation with %d reservations: %v", concurrent, err)
	}
	for _, reservation := range reservations {
		reservation.Release()
	}

	next, err := h.engine.ReserveLLMInvocation(ctx, "org-1")
	if err != nil {
		t.Fatalf("reservation after release: %v", err)
	}
	next.Release()
}

func TestLLMReservationsUseBoundedEstimate(t *testing.T) {
	plan := Free()
	plan.MonthlyTokenLimit = 100
	tokens := newFakeTokenStore()
	engine := New(Deps{
		Plan:             plan,
		TokenUsage:       tokens,
		Disk:             newFakeDisk(),
		Counts:           newFakeCounts(),
		Clock:            newFakeClock(t, "2026-03-15T10:00:00Z"),
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		LLMReserveTokens: 10,
	})
	ctx := context.Background()
	if err := engine.RecordTokenUsage(ctx, "org-1", 5, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}

	var reservations []ports.LLMReservation
	for {
		reservation, err := engine.ReserveLLMInvocation(ctx, "org-1")
		if errors.Is(err, ports.ErrNoCredits) {
			break
		}
		if err != nil {
			t.Fatalf("ReserveLLMInvocation: %v", err)
		}
		reservations = append(reservations, reservation)
	}
	if got, wantAtLeast := len(reservations), 95/10; got < wantAtLeast {
		t.Fatalf("simultaneous reservations = %d, want at least %d", got, wantAtLeast)
	}
	if got := len(reservations); got != 10 {
		t.Fatalf("simultaneous reservations = %d, want 10 including the final 5-token remainder", got)
	}
	for _, reservation := range reservations {
		reservation.Release()
	}
}

func TestLLMReservationSettleFailureReleasesAdmission(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	reservation, err := h.engine.ReserveLLMInvocation(ctx, "org-1")
	if err != nil {
		t.Fatalf("reservation: %v", err)
	}
	h.tokens.err = errors.New("write failed")
	if err := reservation.Settle(ctx, 1, 1); err == nil {
		t.Fatal("Settle = nil, want store error")
	}
	h.tokens.err = nil

	next, err := h.engine.ReserveLLMInvocation(ctx, "org-1")
	if err != nil {
		t.Fatalf("reservation after settle failure: %v", err)
	}
	next.Release()
}

func TestRecordTokenUsageRejectsNegativeValues(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	tests := []struct {
		name   string
		input  int64
		output int64
	}{
		{name: "negative input", input: -1},
		{name: "negative output", output: -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := h.engine.RecordTokenUsage(ctx, "org-1", tc.input, tc.output); !errors.Is(err, ports.ErrInvalidTokenUsage) {
				t.Fatalf("RecordTokenUsage = %v, want ErrInvalidTokenUsage", err)
			}
		})
	}

	meters, err := h.engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.TokensUsed != 0 {
		t.Fatalf("TokensUsed = %d, want 0", meters.TokensUsed)
	}
}

func TestStructuralCaps(t *testing.T) {
	plan := Free()

	tests := []struct {
		name    string
		count   int
		check   func(*Engine, context.Context) error
		set     func(*fakeCounts, int)
		wantErr error
	}{
		{
			name:  "5th member allowed",
			count: plan.MaxMembersPerOrg - 1,
			check: func(e *Engine, ctx context.Context) error { return e.CanAddMember(ctx, "org-1") },
			set:   func(c *fakeCounts, n int) { c.members["org-1"] = n },
		},
		{
			name:    "6th member rejected",
			count:   plan.MaxMembersPerOrg,
			check:   func(e *Engine, ctx context.Context) error { return e.CanAddMember(ctx, "org-1") },
			set:     func(c *fakeCounts, n int) { c.members["org-1"] = n },
			wantErr: ports.ErrMemberCapReached,
		},
		{
			name:  "1st owned org allowed",
			count: plan.MaxOwnedOrgsPerUser - 1,
			check: func(e *Engine, ctx context.Context) error { return e.CanCreateOrg(ctx, "user-1") },
			set:   func(c *fakeCounts, n int) { c.orgs["user-1"] = n },
		},
		{
			name:    "2nd owned org rejected",
			count:   plan.MaxOwnedOrgsPerUser,
			check:   func(e *Engine, ctx context.Context) error { return e.CanCreateOrg(ctx, "user-1") },
			set:     func(c *fakeCounts, n int) { c.orgs["user-1"] = n },
			wantErr: ports.ErrOrgCapReached,
		},
		{
			name:  "10th project allowed",
			count: plan.MaxProjectsPerOrg - 1,
			check: func(e *Engine, ctx context.Context) error { return e.CanCreateProject(ctx, "org-1") },
			set:   func(c *fakeCounts, n int) { c.projects["org-1"] = n },
		},
		{
			name:    "11th project rejected",
			count:   plan.MaxProjectsPerOrg,
			check:   func(e *Engine, ctx context.Context) error { return e.CanCreateProject(ctx, "org-1") },
			set:     func(c *fakeCounts, n int) { c.projects["org-1"] = n },
			wantErr: ports.ErrProjectCapReached,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			tc.set(h.counts, tc.count)
			if err := tc.check(h.engine, context.Background()); !errors.Is(err, tc.wantErr) {
				t.Fatalf("check = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestStructuralAdmissionHeldAcrossMutation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.counts.orgs["user-1"] = h.plan.MaxOwnedOrgsPerUser - 1

	first, err := h.engine.ReserveOrgSlot(ctx, "user-1")
	if err != nil {
		t.Fatalf("first reservation: %v", err)
	}

	result := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		admission, err := h.engine.ReserveOrgSlot(ctx, "user-1")
		if admission != nil {
			admission.Release()
		}
		result <- err
	}()
	<-started

	select {
	case err := <-result:
		t.Fatalf("second admission returned before first mutation completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	h.counts.orgs["user-1"]++
	first.Release()
	if err := <-result; !errors.Is(err, ports.ErrOrgCapReached) {
		t.Fatalf("second admission = %v, want ErrOrgCapReached", err)
	}
}

func TestMetersReportUsageAgainstPlan(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.disk.setOrg("org-1", 512*1024*1024)
	if err := h.engine.RecordTokenUsage(ctx, "org-1", 1_000, 2_000); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}

	meters, err := h.engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}

	want := ports.Meters{
		OrgID:          "org-1",
		Plan:           h.plan.Name,
		Month:          "2026-03",
		DiskUsedBytes:  512 * 1024 * 1024,
		DiskLimitBytes: h.plan.DiskLimitBytes,
		TokensUsed:     3_000,
		TokensLimit:    h.plan.MonthlyTokenLimit,
		MemberLimit:    h.plan.MaxMembersPerOrg,
		OrgLimit:       h.plan.MaxOwnedOrgsPerUser,
		ProjectLimit:   h.plan.MaxProjectsPerOrg,
		MeasuredAt:     h.clock.Now(),
	}
	if !reflect.DeepEqual(meters, want) {
		t.Fatalf("Meters = %+v, want %+v", meters, want)
	}
}

func TestMeterEventsRiseOnceAndRearm(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	limit := h.plan.DiskLimitBytes

	steps := []struct {
		usage int64
		want  []int
	}{
		{usage: limit / 2, want: nil},
		{usage: atLeastPct(limit, 80), want: []int{80}},
		{usage: limit * 85 / 100, want: nil},
		{usage: limit * 96 / 100, want: []int{95}},
		{usage: limit, want: []int{100}},
		{usage: limit + 1, want: nil},
	}
	for _, step := range steps {
		h.events.reset()
		h.disk.setOrg("org-1", step.usage)
		for range 3 { // repeated polls must not re-fire
			if _, err := h.engine.Meters(ctx, "org-1"); err != nil {
				t.Fatalf("Meters: %v", err)
			}
		}
		if got := h.events.thresholds(ports.MeterDisk); !reflect.DeepEqual(got, step.want) {
			t.Fatalf("usage %d fired %v, want %v", step.usage, got, step.want)
		}
	}

	// Falling below 80% re-arms every threshold, so climbing again re-fires.
	h.events.reset()
	h.disk.setOrg("org-1", limit/2)
	if _, err := h.engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if got := h.events.thresholds(ports.MeterDisk); got != nil {
		t.Fatalf("falling usage fired %v, want none", got)
	}

	h.disk.setOrg("org-1", limit)
	if _, err := h.engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if got, want := h.events.thresholds(ports.MeterDisk), []int{80, 95, 100}; !reflect.DeepEqual(got, want) {
		t.Fatalf("re-climb fired %v, want %v", got, want)
	}
}

// atLeastPct is the smallest byte count that measures as pct of limit under
// the engine's integer arithmetic.
func atLeastPct(limit int64, pct int64) int64 {
	return (limit*pct + 99) / 100
}

func TestTokenMeterEventsPerMonth(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	limit := h.plan.MonthlyTokenLimit

	if err := h.engine.RecordTokenUsage(ctx, "org-1", limit*80/100, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}
	if err := h.engine.RecordTokenUsage(ctx, "org-1", 1, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}
	if got, want := h.events.thresholds(ports.MeterTokens), []int{80}; !reflect.DeepEqual(got, want) {
		t.Fatalf("fired %v, want %v", got, want)
	}

	events := h.events.snapshot()
	if last := events[len(events)-1]; last.Period != "2026-03" || last.OrgID != "org-1" || last.Limit != limit {
		t.Fatalf("event = %+v, want org-1 in 2026-03 against the plan limit", last)
	}

	h.events.reset()
	h.clock.set(t, "2026-04-02T00:00:00Z")
	if err := h.engine.RecordTokenUsage(ctx, "org-1", limit, 0); err != nil {
		t.Fatalf("RecordTokenUsage: %v", err)
	}
	if got, want := h.events.thresholds(ports.MeterTokens), []int{80, 95, 100}; !reflect.DeepEqual(got, want) {
		t.Fatalf("new month fired %v, want %v", got, want)
	}
}

func TestWatcherMeasuresAndStops(t *testing.T) {
	clk := newFakeClock(t, "2026-03-15T10:00:00Z")
	disk := newFakeDisk()
	events := &recorder{}
	plan := Free()

	engine := New(Deps{
		Plan:             plan,
		TokenUsage:       newFakeTokenStore(),
		Disk:             disk,
		Counts:           newFakeCounts(),
		Clock:            clk,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		WatchInterval:    time.Millisecond,
		HostWatermarkPct: 80,
	})
	engine.Subscribe(events.record)

	disk.setOrg("org-1", plan.DiskLimitBytes)
	disk.setHost(1, 100)

	ctx, cancel := context.WithCancel(context.Background())
	engine.Start(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(events.thresholds(ports.MeterDisk)) == 3 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			engine.Wait()
			t.Fatalf("watcher fired %v, want 80/95/100", events.thresholds(ports.MeterDisk))
		}
		time.Sleep(time.Millisecond)
	}

	cancel()
	engine.Wait()

	// The watcher measured the host too, so the global switch is on.
	if err := engine.CheckDiskWrite(context.Background(), "org-2", 1); !errors.Is(err, ports.ErrOutOfSpace) {
		t.Fatalf("CheckDiskWrite = %v, want ErrOutOfSpace", err)
	}

	before := len(events.snapshot())
	time.Sleep(20 * time.Millisecond)
	if after := len(events.snapshot()); after != before {
		t.Fatalf("watcher emitted %d events after stop, want none", after-before)
	}
}

func TestZeroCountsNeverRejects(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	engine := New(Deps{
		Plan:       h.plan,
		TokenUsage: h.tokens,
		Disk:       h.disk,
		Counts:     ZeroCounts{},
		Clock:      h.clock,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	if err := engine.CanAddMember(ctx, "org-1"); err != nil {
		t.Errorf("CanAddMember: %v", err)
	}
	if err := engine.CanCreateOrg(ctx, "user-1"); err != nil {
		t.Errorf("CanCreateOrg: %v", err)
	}
	if err := engine.CanCreateProject(ctx, "org-1"); err != nil {
		t.Errorf("CanCreateProject: %v", err)
	}
}

func TestDiskMeasurementCachedUntilStale(t *testing.T) {
	clk := newFakeClock(t, "2026-03-15T10:00:00Z")
	disk := newFakeDisk()
	engine := New(Deps{
		Plan:             Free(),
		TokenUsage:       newFakeTokenStore(),
		Disk:             disk,
		Counts:           newFakeCounts(),
		Clock:            clk,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		WatchInterval:    30 * time.Second,
		HostWatermarkPct: 80,
	})
	ctx := context.Background()

	disk.setOrg("org-1", 1_000)
	if _, err := engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}

	disk.setOrg("org-1", 2_000)
	meters, err := engine.Meters(ctx, "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.DiskUsedBytes != 1_000 {
		t.Fatalf("DiskUsedBytes = %d, want the cached 1000", meters.DiskUsedBytes)
	}

	clk.advance(31 * time.Second)
	if meters, err = engine.Meters(ctx, "org-1"); err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.DiskUsedBytes != 2_000 {
		t.Fatalf("DiskUsedBytes = %d, want the re-measured 2000", meters.DiskUsedBytes)
	}
}

func TestStaleDiskMeasurementDoesNotReplaceNewerValue(t *testing.T) {
	clk := newFakeClock(t, "2026-03-15T10:00:00Z")
	engine := New(Deps{
		Plan:             Free(),
		TokenUsage:       newFakeTokenStore(),
		Disk:             newFakeDisk(),
		Counts:           newFakeCounts(),
		Clock:            clk,
		Logger:           slog.New(slog.NewTextHandler(io.Discard, nil)),
		WatchInterval:    time.Hour,
		HostWatermarkPct: 80,
	})
	newer := clk.Now()
	older := newer.Add(-time.Minute)
	engine.applyOrgDisk("org-1", engine.plan.DiskLimitBytes, newer)
	engine.applyOrgDisk("org-1", 0, older)

	if err := engine.CheckDiskWrite(context.Background(), "org-1", 1); !errors.Is(err, ports.ErrOrgReadOnly) {
		t.Fatalf("CheckDiskWrite after stale measurement = %v, want ErrOrgReadOnly", err)
	}
	meters, err := engine.Meters(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("Meters: %v", err)
	}
	if meters.DiskUsedBytes != engine.plan.DiskLimitBytes || !meters.MeasuredAt.Equal(newer) {
		t.Fatalf("Meters = %+v, want newer at-cap measurement", meters)
	}
}

func TestTokenMonthNeverMovesBackward(t *testing.T) {
	h := newHarness(t)
	limit := h.plan.MonthlyTokenLimit

	if got := h.engine.applyTokenUsage("org-1", "2026-04", limit); len(got) != 3 {
		t.Fatalf("April crossings = %d, want 3", len(got))
	}
	if got := h.engine.applyTokenUsage("org-1", "2026-03", 0); got != nil {
		t.Fatalf("stale March usage emitted %v, want none", got)
	}
	if got := h.engine.applyTokenUsage("org-1", "2026-04", limit); got != nil {
		t.Fatalf("April threshold re-fired after stale month: %v", got)
	}

	h.engine.mu.Lock()
	month := h.engine.stateLocked("org-1").tokenMonth
	h.engine.mu.Unlock()
	if month != "2026-04" {
		t.Fatalf("tokenMonth = %q, want 2026-04", month)
	}
}

func TestOverflowSafeAuthorityComparisons(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()

	h.disk.setOrg("org-1", h.plan.DiskLimitBytes-1)
	if err := h.engine.CheckDiskWrite(ctx, "org-1", math.MaxInt64); !errors.Is(err, ports.ErrDiskQuotaExceeded) {
		t.Fatalf("overflowing disk delta = %v, want ErrDiskQuotaExceeded", err)
	}

	h.tokens.usage[usageKey{orgID: "org-2", month: "2026-03"}] = [2]int64{math.MaxInt64, math.MaxInt64}
	if err := h.engine.CheckLLMInvocation(ctx, "org-2"); !errors.Is(err, ports.ErrNoCredits) {
		t.Fatalf("overflowing token total = %v, want ErrNoCredits", err)
	}

	if got := thresholdLevel(math.MaxInt64, math.MaxInt64); got != len(ports.MeterThresholds) {
		t.Fatalf("thresholdLevel(MaxInt64, MaxInt64) = %d, want %d", got, len(ports.MeterThresholds))
	}

	h.disk.setHost(0, math.MaxInt64)
	if err := h.engine.CheckDiskWrite(ctx, "org-3", 1); !errors.Is(err, ports.ErrOutOfSpace) {
		t.Fatalf("MaxInt64 host usage = %v, want ErrOutOfSpace", err)
	}
}

func TestThresholdLevel(t *testing.T) {
	tests := []struct {
		used, limit int64
		want        int
	}{
		{0, 100, 0},
		{79, 100, 0},
		{80, 100, 1},
		{94, 100, 1},
		{95, 100, 2},
		{99, 100, 2},
		{100, 100, 3},
		{101, 100, 3},
		{10, 0, 0},
	}
	for _, tc := range tests {
		if got := thresholdLevel(tc.used, tc.limit); got != tc.want {
			t.Errorf("thresholdLevel(%d, %d) = %d, want %d", tc.used, tc.limit, got, tc.want)
		}
	}
}
