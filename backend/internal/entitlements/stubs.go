package entitlements

import "context"

// ZeroCounts is a placeholder StructuralCounts that reports every count as
// zero, so the structural caps never reject. It is wired at boot until the
// organization data store lands.
type ZeroCounts struct{}

var _ StructuralCounts = ZeroCounts{}

func (ZeroCounts) MemberCount(context.Context, string) (int, error)   { return 0, nil }
func (ZeroCounts) OwnedOrgCount(context.Context, string) (int, error) { return 0, nil }
func (ZeroCounts) ProjectCount(context.Context, string) (int, error)  { return 0, nil }
