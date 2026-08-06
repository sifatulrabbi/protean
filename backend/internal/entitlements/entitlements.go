// Package entitlements is the pricing engine: it holds the plan definitions
// and answers whether an operation is inside the org's plan. It implements
// ports.Entitlements and depends only on the small stores declared here.
package entitlements

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// TokenUsageStore persists AI token counters, keyed by org and calendar month
// ("YYYY-MM"). Counters only ever grow inside a month; a new month starts at
// zero because it is a new key.
type TokenUsageStore interface {
	AddUsage(ctx context.Context, orgID, month string, inputTokens, outputTokens int64) error
	UsageForMonth(ctx context.Context, orgID, month string) (inputTokens, outputTokens int64, err error)
}

// DiskUsage measures bytes on the host.
type DiskUsage interface {
	// OrgUsageBytes is the size of the whole org tree. A missing tree is 0.
	OrgUsageBytes(ctx context.Context, orgID string) (int64, error)
	// HostDisk reports the filesystem holding the data directory.
	HostDisk(ctx context.Context) (freeBytes, totalBytes int64, err error)
	// ListOrgs enumerates the orgs that have a tree on disk, so the watcher
	// knows what to measure without a database.
	ListOrgs(ctx context.Context) ([]string, error)
}

// StructuralCounts reports the counts behind the structural caps. It is backed
// by the real data store in a later slice.
type StructuralCounts interface {
	MemberCount(ctx context.Context, orgID string) (int, error)
	OwnedOrgCount(ctx context.Context, userID string) (int, error)
	ProjectCount(ctx context.Context, orgID string) (int, error)
}

// Deps are the engine's injected collaborators and tunables.
type Deps struct {
	Plan             Plan
	TokenUsage       TokenUsageStore
	Disk             DiskUsage
	Counts           StructuralCounts
	Clock            ports.Clock
	Logger           *slog.Logger
	WatchInterval    time.Duration
	HostWatermarkPct int
}

// Engine implements ports.Entitlements. Switch state lives in memory; only the
// token counters are persisted.
type Engine struct {
	plan         Plan
	tokens       TokenUsageStore
	disk         DiskUsage
	counts       StructuralCounts
	clock        ports.Clock
	logger       *slog.Logger
	interval     time.Duration
	watermarkPct int

	mu         sync.Mutex
	outOfSpace bool
	orgs       map[string]*orgState
	subs       []func(ports.MeterEvent)

	wg sync.WaitGroup
}

// orgState is the cached, mutable per-org view the switches are derived from.
type orgState struct {
	diskBytes  int64
	measuredAt time.Time
	// diskLevel and tokenLevel are how many thresholds the org has already
	// fired, so a rising crossing fires once and a fall re-arms it.
	diskLevel  int
	tokenMonth string
	tokenLevel int
}

var _ ports.Entitlements = (*Engine)(nil)

func New(deps Deps) *Engine {
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	plan := deps.Plan
	if plan.Name == "" {
		plan = Free()
	}
	interval := deps.WatchInterval
	if interval < 0 {
		interval = 0
	}
	watermark := deps.HostWatermarkPct
	if watermark <= 0 || watermark > 100 {
		watermark = 100
	}

	return &Engine{
		plan:         plan,
		tokens:       deps.TokenUsage,
		disk:         deps.Disk,
		counts:       deps.Counts,
		clock:        deps.Clock,
		logger:       logger,
		interval:     interval,
		watermarkPct: watermark,
		orgs:         map[string]*orgState{},
	}
}

// Plan exposes the enforced plan, so callers can render limits instead of
// hardcoding them.
func (e *Engine) Plan() Plan { return e.plan }

func (e *Engine) Subscribe(fn func(ports.MeterEvent)) {
	if fn == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.subs = append(e.subs, fn)
}

func (e *Engine) CheckDiskWrite(ctx context.Context, orgID string, deltaBytes int64) error {
	if deltaBytes < 0 {
		return nil
	}

	used, err := e.diskUsed(ctx, orgID)
	if err != nil {
		return err
	}

	e.mu.Lock()
	outOfSpace := e.outOfSpace
	e.mu.Unlock()

	switch {
	case used >= e.plan.DiskLimitBytes:
		return ports.ErrOrgReadOnly
	case used+deltaBytes > e.plan.DiskLimitBytes:
		return ports.ErrDiskQuotaExceeded
	case outOfSpace:
		return ports.ErrOutOfSpace
	}
	return nil
}

func (e *Engine) CheckLLMInvocation(ctx context.Context, orgID string) error {
	month := e.month()
	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	if in+out >= e.plan.MonthlyTokenLimit {
		return ports.ErrNoCredits
	}
	return nil
}

func (e *Engine) RecordTokenUsage(ctx context.Context, orgID string, inputTokens, outputTokens int64) error {
	month := e.month()
	if err := e.tokens.AddUsage(ctx, orgID, month, inputTokens, outputTokens); err != nil {
		return fmt.Errorf("record token usage for %s/%s: %w", orgID, month, err)
	}

	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	e.emit(e.applyTokenUsage(orgID, month, in+out))
	return nil
}

func (e *Engine) CanAddMember(ctx context.Context, orgID string) error {
	n, err := e.counts.MemberCount(ctx, orgID)
	if err != nil {
		return fmt.Errorf("member count for %s: %w", orgID, err)
	}
	if n >= e.plan.MaxMembersPerOrg {
		return ports.ErrMemberCapReached
	}
	return nil
}

func (e *Engine) CanCreateOrg(ctx context.Context, userID string) error {
	n, err := e.counts.OwnedOrgCount(ctx, userID)
	if err != nil {
		return fmt.Errorf("owned org count for %s: %w", userID, err)
	}
	if n >= e.plan.MaxOwnedOrgsPerUser {
		return ports.ErrOrgCapReached
	}
	return nil
}

func (e *Engine) CanCreateProject(ctx context.Context, orgID string) error {
	n, err := e.counts.ProjectCount(ctx, orgID)
	if err != nil {
		return fmt.Errorf("project count for %s: %w", orgID, err)
	}
	if n >= e.plan.MaxProjectsPerOrg {
		return ports.ErrProjectCapReached
	}
	return nil
}

func (e *Engine) Meters(ctx context.Context, orgID string) (ports.Meters, error) {
	used, err := e.diskUsed(ctx, orgID)
	if err != nil {
		return ports.Meters{}, err
	}

	month := e.month()
	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return ports.Meters{}, fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	tokens := in + out

	e.mu.Lock()
	outOfSpace := e.outOfSpace
	measuredAt := e.orgs[orgID].measuredAt
	e.mu.Unlock()

	return ports.Meters{
		OrgID:          orgID,
		Plan:           e.plan.Name,
		Month:          month,
		DiskUsedBytes:  used,
		DiskLimitBytes: e.plan.DiskLimitBytes,
		TokensUsed:     tokens,
		TokensLimit:    e.plan.MonthlyTokenLimit,
		MemberLimit:    e.plan.MaxMembersPerOrg,
		OrgLimit:       e.plan.MaxOwnedOrgsPerUser,
		ProjectLimit:   e.plan.MaxProjectsPerOrg,
		OrgReadOnly:    used >= e.plan.DiskLimitBytes,
		OutOfSpace:     outOfSpace,
		NoCredits:      tokens >= e.plan.MonthlyTokenLimit,
		MeasuredAt:     measuredAt,
	}, nil
}

// Start runs the measurement watcher until ctx is cancelled. Wait blocks until
// it has stopped.
func (e *Engine) Start(ctx context.Context) {
	interval := e.interval
	if interval <= 0 {
		interval = time.Second
	}

	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		e.refresh(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				e.refresh(ctx)
			}
		}
	}()
}

func (e *Engine) Wait() { e.wg.Wait() }

// refresh re-measures the host and every org on disk, updates the switches and
// emits the meter events the new measurements cause.
func (e *Engine) refresh(ctx context.Context) {
	e.refreshHost(ctx)

	orgIDs, err := e.disk.ListOrgs(ctx)
	if err != nil {
		e.logger.Error("entitlements: list orgs", "err", err)
		return
	}

	now := e.clock.Now()
	for _, orgID := range orgIDs {
		used, err := e.disk.OrgUsageBytes(ctx, orgID)
		if err != nil {
			e.logger.Error("entitlements: measure org disk", "org_id", orgID, "err", err)
			continue
		}
		e.emit(e.applyOrgDisk(orgID, used, now))
	}
}

func (e *Engine) refreshHost(ctx context.Context) {
	free, total, err := e.disk.HostDisk(ctx)
	if err != nil {
		e.logger.Error("entitlements: measure host disk", "err", err)
		return
	}
	if total <= 0 {
		return
	}

	usedPct := (total - free) * 100 / total
	on := usedPct >= int64(e.watermarkPct)

	e.mu.Lock()
	changed := e.outOfSpace != on
	e.outOfSpace = on
	e.mu.Unlock()

	if changed {
		e.logger.Warn("entitlements: out-of-space switch changed",
			"on", on, "host_used_pct", usedPct, "watermark_pct", e.watermarkPct)
	}
}

// diskUsed returns the org's cached disk usage, re-measuring first when the
// cached value is older than the watch interval.
func (e *Engine) diskUsed(ctx context.Context, orgID string) (int64, error) {
	now := e.clock.Now()

	e.mu.Lock()
	st := e.stateLocked(orgID)
	fresh := !st.measuredAt.IsZero() && now.Sub(st.measuredAt) < e.interval
	used := st.diskBytes
	e.mu.Unlock()

	if fresh {
		return used, nil
	}

	used, err := e.disk.OrgUsageBytes(ctx, orgID)
	if err != nil {
		return 0, fmt.Errorf("measure disk usage for %s: %w", orgID, err)
	}
	e.emit(e.applyOrgDisk(orgID, used, now))
	e.refreshHost(ctx)
	return used, nil
}

func (e *Engine) applyOrgDisk(orgID string, used int64, now time.Time) []ports.MeterEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.stateLocked(orgID)
	wasReadOnly := st.diskBytes >= e.plan.DiskLimitBytes && !st.measuredAt.IsZero()
	st.diskBytes = used
	st.measuredAt = now

	if isReadOnly := used >= e.plan.DiskLimitBytes; isReadOnly != wasReadOnly {
		e.logger.Warn("entitlements: org read-only switch changed",
			"org_id", orgID, "read_only", isReadOnly, "used_bytes", used, "limit_bytes", e.plan.DiskLimitBytes)
	}

	events, level := crossings(orgID, ports.MeterDisk, "", used, e.plan.DiskLimitBytes, st.diskLevel, now)
	st.diskLevel = level
	return events
}

func (e *Engine) applyTokenUsage(orgID, month string, used int64) []ports.MeterEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.stateLocked(orgID)
	if st.tokenMonth != month {
		st.tokenMonth = month
		st.tokenLevel = 0
	}

	events, level := crossings(orgID, ports.MeterTokens, month, used, e.plan.MonthlyTokenLimit, st.tokenLevel, e.clock.Now())
	st.tokenLevel = level
	return events
}

func (e *Engine) stateLocked(orgID string) *orgState {
	st, ok := e.orgs[orgID]
	if !ok {
		st = &orgState{}
		e.orgs[orgID] = st
	}
	return st
}

// crossings reports the events for moving from firedLevel thresholds to the
// level the usage now sits at, and the new level. Falling usage emits nothing
// but lowers the level, which re-arms the thresholds it dropped below.
func crossings(orgID string, resource ports.MeterResource, period string, used, limit int64, firedLevel int, at time.Time) ([]ports.MeterEvent, int) {
	level := thresholdLevel(used, limit)
	if level <= firedLevel {
		return nil, level
	}

	events := make([]ports.MeterEvent, 0, level-firedLevel)
	for i := firedLevel; i < level; i++ {
		events = append(events, ports.MeterEvent{
			OrgID:        orgID,
			Resource:     resource,
			ThresholdPct: ports.MeterThresholds[i],
			Used:         used,
			Limit:        limit,
			Period:       period,
			At:           at,
		})
	}
	return events, level
}

// thresholdLevel is how many of ports.MeterThresholds the usage has reached.
func thresholdLevel(used, limit int64) int {
	if limit <= 0 || used <= 0 {
		return 0
	}
	pct := used * 100 / limit
	level := 0
	for _, t := range ports.MeterThresholds {
		if pct >= int64(t) {
			level++
		}
	}
	return level
}

func (e *Engine) emit(events []ports.MeterEvent) {
	if len(events) == 0 {
		return
	}

	e.mu.Lock()
	subs := make([]func(ports.MeterEvent), len(e.subs))
	copy(subs, e.subs)
	e.mu.Unlock()

	for _, ev := range events {
		e.logger.Info("entitlements: meter threshold crossed",
			"org_id", ev.OrgID, "resource", ev.Resource, "threshold_pct", ev.ThresholdPct,
			"used", ev.Used, "limit", ev.Limit, "period", ev.Period)
		for _, fn := range subs {
			fn(ev)
		}
	}
}

func (e *Engine) month() string { return e.clock.Now().UTC().Format("2006-01") }
