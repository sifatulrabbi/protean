// Package entitlements is the pricing engine: it holds the plan definitions
// and answers whether an operation is inside the org's plan. It implements
// ports.Entitlements and depends only on the small stores declared here.
package entitlements

import (
	"context"
	"fmt"
	"log/slog"
	"math/bits"
	"sync"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

const defaultLLMReserveTokens int64 = 50_000

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
	LLMReserveTokens int64
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
	llmReserve   int64

	mu         sync.Mutex
	outOfSpace bool
	orgs       map[string]*orgState
	subs       []func(ports.MeterEvent)

	tokenMu       sync.Mutex
	tokenReserved map[tokenScope]int64

	admissionMu    sync.Mutex
	admissionLocks map[structuralScope]chan struct{}

	wg sync.WaitGroup
}

type tokenScope struct {
	orgID string
	month string
}

type structuralScope struct {
	kind string
	id   string
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
var _ ports.ReservingEntitlements = (*Engine)(nil)

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
	llmReserve := deps.LLMReserveTokens
	if llmReserve <= 0 {
		llmReserve = defaultLLMReserveTokens
	}

	return &Engine{
		plan:           plan,
		tokens:         deps.TokenUsage,
		disk:           deps.Disk,
		counts:         deps.Counts,
		clock:          deps.Clock,
		logger:         logger,
		interval:       interval,
		watermarkPct:   watermark,
		llmReserve:     llmReserve,
		orgs:           map[string]*orgState{},
		tokenReserved:  map[tokenScope]int64{},
		admissionLocks: map[structuralScope]chan struct{}{},
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
	case deltaBytes > e.plan.DiskLimitBytes-used:
		return ports.ErrDiskQuotaExceeded
	case outOfSpace:
		return ports.ErrOutOfSpace
	}
	return nil
}

func (e *Engine) CheckLLMInvocation(ctx context.Context, orgID string) error {
	month := e.month()
	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	used := saturatingAddNonNegative(in, out)
	reserved := e.tokenReserved[tokenScope{orgID: orgID, month: month}]
	if sumAtLeastLimit(used, reserved, e.plan.MonthlyTokenLimit) {
		return ports.ErrNoCredits
	}
	return nil
}

func (e *Engine) ReserveLLMInvocation(ctx context.Context, orgID string) (ports.LLMReservation, error) {
	month := e.month()
	scope := tokenScope{orgID: orgID, month: month}

	e.tokenMu.Lock()
	defer e.tokenMu.Unlock()

	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return nil, fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	used := saturatingAddNonNegative(in, out)
	reserved := e.tokenReserved[scope]
	if sumAtLeastLimit(used, reserved, e.plan.MonthlyTokenLimit) {
		return nil, ports.ErrNoCredits
	}

	// Reserve a bounded estimate while budget is plentiful. The final admission
	// takes the smaller remainder so no later invocation can pass the cap.
	remaining := e.plan.MonthlyTokenLimit - used - reserved
	amount := min(remaining, e.llmReserve)
	e.tokenReserved[scope] = reserved + amount
	return &llmReservation{engine: e, scope: scope, amount: amount}, nil
}

func (e *Engine) RecordTokenUsage(ctx context.Context, orgID string, inputTokens, outputTokens int64) error {
	if inputTokens < 0 || outputTokens < 0 {
		return ports.ErrInvalidTokenUsage
	}

	month := e.month()
	e.tokenMu.Lock()
	used, err := e.addTokenUsageLocked(ctx, orgID, month, inputTokens, outputTokens)
	e.tokenMu.Unlock()
	if err != nil {
		return err
	}
	e.emit(e.applyTokenUsage(orgID, month, used))
	return nil
}

func (e *Engine) addTokenUsageLocked(ctx context.Context, orgID, month string, inputTokens, outputTokens int64) (int64, error) {
	if err := e.tokens.AddUsage(ctx, orgID, month, inputTokens, outputTokens); err != nil {
		return 0, fmt.Errorf("record token usage for %s/%s: %w", orgID, month, err)
	}

	in, out, err := e.tokens.UsageForMonth(ctx, orgID, month)
	if err != nil {
		return 0, fmt.Errorf("token usage for %s/%s: %w", orgID, month, err)
	}
	return saturatingAddNonNegative(in, out), nil
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

func (e *Engine) ReserveMemberSlot(ctx context.Context, orgID string) (ports.StructuralAdmission, error) {
	return e.reserveStructural(ctx, structuralScope{kind: "member", id: orgID}, func() error {
		return e.CanAddMember(ctx, orgID)
	})
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

func (e *Engine) ReserveOrgSlot(ctx context.Context, userID string) (ports.StructuralAdmission, error) {
	return e.reserveStructural(ctx, structuralScope{kind: "org", id: userID}, func() error {
		return e.CanCreateOrg(ctx, userID)
	})
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

func (e *Engine) ReserveProjectSlot(ctx context.Context, orgID string) (ports.StructuralAdmission, error) {
	return e.reserveStructural(ctx, structuralScope{kind: "project", id: orgID}, func() error {
		return e.CanCreateProject(ctx, orgID)
	})
}

type llmReservation struct {
	engine *Engine
	scope  tokenScope
	amount int64

	mu     sync.Mutex
	closed bool
}

func (r *llmReservation) Settle(ctx context.Context, inputTokens, outputTokens int64) error {
	if inputTokens < 0 || outputTokens < 0 {
		return ports.ErrInvalidTokenUsage
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return fmt.Errorf("LLM reservation is already closed")
	}

	r.engine.tokenMu.Lock()
	used, err := r.engine.addTokenUsageLocked(ctx, r.scope.orgID, r.scope.month, inputTokens, outputTokens)
	r.engine.releaseTokenReservationLocked(r.scope, r.amount)
	r.engine.tokenMu.Unlock()
	r.closed = true
	if err != nil {
		return err
	}
	r.engine.emit(r.engine.applyTokenUsage(r.scope.orgID, r.scope.month, used))
	return nil
}

func (r *llmReservation) Release() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}

	r.engine.tokenMu.Lock()
	r.engine.releaseTokenReservationLocked(r.scope, r.amount)
	r.engine.tokenMu.Unlock()
	r.closed = true
}

func (e *Engine) releaseTokenReservationLocked(scope tokenScope, amount int64) {
	reserved := e.tokenReserved[scope]
	if reserved <= amount {
		delete(e.tokenReserved, scope)
		return
	}
	e.tokenReserved[scope] = reserved - amount
}

type structuralAdmission struct {
	once sync.Once
	lock chan struct{}
}

func (a *structuralAdmission) Release() {
	a.once.Do(func() { a.lock <- struct{}{} })
}

func (e *Engine) reserveStructural(ctx context.Context, scope structuralScope, check func() error) (ports.StructuralAdmission, error) {
	e.admissionMu.Lock()
	lock, ok := e.admissionLocks[scope]
	if !ok {
		lock = make(chan struct{}, 1)
		lock <- struct{}{}
		e.admissionLocks[scope] = lock
	}
	e.admissionMu.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-lock:
	}

	admission := &structuralAdmission{lock: lock}
	if err := check(); err != nil {
		admission.Release()
		return nil, err
	}
	return admission, nil
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
	tokens := saturatingAddNonNegative(in, out)

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

	var used int64
	switch {
	case free <= 0:
		used = total
	case free < total:
		used = total - free
	}
	usedPct := percentage(used, total)
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

	e.mu.Lock()
	used = e.stateLocked(orgID).diskBytes
	e.mu.Unlock()
	return used, nil
}

func (e *Engine) applyOrgDisk(orgID string, used int64, now time.Time) []ports.MeterEvent {
	e.mu.Lock()
	defer e.mu.Unlock()

	st := e.stateLocked(orgID)
	if now.Before(st.measuredAt) {
		return nil
	}
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
	if st.tokenMonth > month {
		return nil
	}
	if st.tokenMonth < month {
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
	level := 0
	for _, t := range ports.MeterThresholds {
		if atLeastPercentage(used, limit, int64(t)) {
			level++
		}
	}
	return level
}

func sumAtLeastLimit(a, b, limit int64) bool {
	if limit <= 0 || a >= limit || b >= limit {
		return true
	}
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	return b >= limit-a
}

func saturatingAddNonNegative(a, b int64) int64 {
	if a < 0 {
		a = 0
	}
	if b < 0 {
		b = 0
	}
	if b > int64(^uint64(0)>>1)-a {
		return int64(^uint64(0) >> 1)
	}
	return a + b
}

func atLeastPercentage(used, total, pct int64) bool {
	if used <= 0 || total <= 0 || pct <= 0 {
		return false
	}
	if used >= total || pct >= 100 {
		return used >= total
	}
	whole := total / 100
	remainder := total % 100
	threshold := whole*pct + (remainder*pct+99)/100
	return used >= threshold
}

func percentage(used, total int64) int64 {
	if used <= 0 || total <= 0 {
		return 0
	}
	if used >= total {
		return 100
	}
	hi, lo := bits.Mul64(uint64(used), 100)
	pct, _ := bits.Div64(hi, lo, uint64(total))
	return int64(pct)
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
