package ports

import (
	"context"
	"time"
)

// Entitlements is the plan-enforcement port. It is the single authority on
// limits: no other package may hardcode a quota value. Callers ask before they
// act, and act on the verdict.
//
// Enforcement is strict but advisory in mechanism (D17): the engine only
// returns a verdict, it never rolls anything back. A caller that gets an error
// from a Check* method must cancel the operation itself and delete whatever
// partial data it already wrote. A caller that ignores the verdict can exceed
// the plan; the watcher will then latch the org read-only until usage drops.
//
// Every Check* method returns nil when the operation is allowed, or an error
// matching one of the sentinels in this file when it is not. Match with
// errors.Is; the message is safe to show to an end user.
type Entitlements interface {
	// CheckDiskWrite is the preflight for any write into the org tree.
	// deltaBytes is the number of bytes the write will add (0 for overwrites
	// that do not grow, negative for deletes, which are always allowed).
	// It fails with ErrOrgReadOnly when the org already sits at its disk cap,
	// ErrDiskQuotaExceeded when the delta would cross the cap, and
	// ErrOutOfSpace when the host itself is running out of room.
	CheckDiskWrite(ctx context.Context, orgID string, deltaBytes int64) error

	// CheckLLMInvocation fails with ErrNoCredits when the org has spent its
	// token allowance for the current calendar month or its remaining allowance
	// is held by an in-flight reservation.
	CheckLLMInvocation(ctx context.Context, orgID string) error

	// RecordTokenUsage adds a completed invocation's tokens to the org's
	// counter for the current calendar month. It is called after the fact and
	// rejects negative usage with ErrInvalidTokenUsage. Overshoot on the last
	// invocation is accepted and the org loses credits until the month rolls
	// over.
	RecordTokenUsage(ctx context.Context, orgID string, inputTokens, outputTokens int64) error

	// CanAddMember fails with ErrMemberCapReached when the org is full.
	CanAddMember(ctx context.Context, orgID string) error

	// CanCreateOrg fails with ErrOrgCapReached when the user already owns the
	// most organizations their plan allows.
	CanCreateOrg(ctx context.Context, userID string) error

	// CanCreateProject fails with ErrProjectCapReached when the org is full.
	CanCreateProject(ctx context.Context, orgID string) error

	// Meters reports current usage against the plan for one org.
	Meters(ctx context.Context, orgID string) (Meters, error)

	// Subscribe registers fn for meter events. fn is called synchronously from
	// whichever goroutine crossed the threshold, so it must not block.
	Subscribe(fn func(MeterEvent))
}

// ReservingEntitlements extends Entitlements with atomic, single-host
// reservation and structural-admission operations. It is a separate extension
// interface so existing Entitlements implementations remain source-compatible.
type ReservingEntitlements interface {
	Entitlements

	// ReserveLLMInvocation atomically holds the org's remaining token allowance
	// for one invocation. The caller must settle the reservation with the
	// provider-reported usage, or release it when the invocation fails.
	ReserveLLMInvocation(ctx context.Context, orgID string) (LLMReservation, error)

	// ReserveMemberSlot checks the member cap while holding per-org admission.
	// The caller must release the handle after its membership mutation commits
	// or fails.
	ReserveMemberSlot(ctx context.Context, orgID string) (StructuralAdmission, error)

	// ReserveOrgSlot checks the owned-org cap while holding per-user admission.
	// The caller must release the handle after its org mutation commits or fails.
	ReserveOrgSlot(ctx context.Context, userID string) (StructuralAdmission, error)

	// ReserveProjectSlot checks the project cap while holding per-org admission.
	// The caller must release the handle after its project mutation commits or
	// fails.
	ReserveProjectSlot(ctx context.Context, orgID string) (StructuralAdmission, error)
}

// LLMReservation is a single-use hold on an org's token allowance. Settle
// records actual provider usage and closes the reservation. Release closes it
// without recording usage.
type LLMReservation interface {
	Settle(ctx context.Context, inputTokens, outputTokens int64) error
	Release()
}

// StructuralAdmission holds a per-scope lock across a structural mutation.
// Release must be called after the mutation commits or fails.
type StructuralAdmission interface {
	Release()
}

// Meters is a point-in-time view of one org's usage against its plan.
type Meters struct {
	OrgID string
	Plan  string

	// Month is the calendar month key ("YYYY-MM") the token counters belong to.
	Month string

	DiskUsedBytes  int64
	DiskLimitBytes int64

	TokensUsed  int64
	TokensLimit int64

	MemberLimit  int
	OrgLimit     int
	ProjectLimit int

	// Switch states, mirroring the sentinel errors.
	OrgReadOnly bool
	OutOfSpace  bool
	NoCredits   bool

	// MeasuredAt is when the disk figure was last measured.
	MeasuredAt time.Time
}

// MeterResource names the metered resource in a MeterEvent.
type MeterResource string

const (
	MeterDisk   MeterResource = "disk"
	MeterTokens MeterResource = "tokens"
)

// MeterEvent is emitted when an org's usage of a resource crosses a usage
// threshold upwards. Events are rising-edge only: repeated measurements above
// the same threshold emit nothing. Usage that falls back below a threshold
// re-arms it, so a later crossing fires again.
// Rising-edge state is intentionally memory-only for this MVP.
type MeterEvent struct {
	OrgID        string
	Resource     MeterResource
	ThresholdPct int
	Used         int64
	Limit        int64

	// Period is the calendar month key for token events, empty for disk.
	Period string
	At     time.Time
}

// MeterThresholds are the usage percentages that emit a MeterEvent.
var MeterThresholds = [...]int{80, 95, 100}

// EntitlementError is the error type every entitlement rejection carries. Code
// is a stable machine-readable identifier; Message is user-facing.
type EntitlementError struct {
	Code    string
	Message string
}

func (e *EntitlementError) Error() string { return e.Code + ": " + e.Message }

// Entitlement rejection sentinels. Match with errors.Is.
var (
	ErrOrgReadOnly = &EntitlementError{
		Code:    "org_read_only",
		Message: "This organization has used all of the storage included in your plan and is now read-only. Delete files or upgrade your plan for more storage.",
	}
	ErrDiskQuotaExceeded = &EntitlementError{
		Code:    "disk_quota_exceeded",
		Message: "This write would exceed the storage included in your plan. Delete files or upgrade your plan for more storage.",
	}
	ErrOutOfSpace = &EntitlementError{
		Code:    "out_of_space",
		Message: "The service is temporarily out of disk space and cannot accept writes. Please retry shortly; upgrade your plan for dedicated capacity.",
	}
	ErrNoCredits = &EntitlementError{
		Code:    "no_credits",
		Message: "This organization has used all of its AI tokens for this month. The allowance resets on the first of next month, or you can upgrade your plan for more.",
	}
	ErrMemberCapReached = &EntitlementError{
		Code:    "member_cap_reached",
		Message: "This organization already has the maximum number of members included in your plan. Upgrade your plan to invite more people.",
	}
	ErrOrgCapReached = &EntitlementError{
		Code:    "org_cap_reached",
		Message: "You already own the maximum number of organizations included in your plan. Upgrade your plan to create another one.",
	}
	ErrProjectCapReached = &EntitlementError{
		Code:    "project_cap_reached",
		Message: "This organization already has the maximum number of projects included in your plan. Delete a project or upgrade your plan for more.",
	}
	ErrInvalidTokenUsage = &EntitlementError{
		Code:    "invalid_token_usage",
		Message: "Token usage cannot be negative.",
	}
)
