package entitlements

import "context"

// ZeroCounts is an UNENFORCED placeholder StructuralCounts. It reports every
// count as zero, so structural caps never reject; production must replace it
// when the metadata store lands.
type ZeroCounts struct{}

var _ StructuralCounts = ZeroCounts{}

func (ZeroCounts) MemberCount(context.Context, string) (int, error)   { return 0, nil }
func (ZeroCounts) OwnedOrgCount(context.Context, string) (int, error) { return 0, nil }
func (ZeroCounts) ProjectCount(context.Context, string) (int, error)  { return 0, nil }
