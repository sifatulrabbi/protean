package entitlements

// Plan is a priced tier. Every limit in Protean lives here and nowhere else:
// no other package may hardcode a quota value.
type Plan struct {
	Name string

	// DiskLimitBytes caps the whole organization tree: every project, thread,
	// memory and skill stored under the org's directory.
	DiskLimitBytes int64

	// MonthlyTokenLimit caps input plus output tokens per calendar month.
	MonthlyTokenLimit int64

	MaxMembersPerOrg    int
	MaxOwnedOrgsPerUser int
	MaxProjectsPerOrg   int
}

const gib = 1 << 30

// Free is the only plan today.
func Free() Plan {
	return Plan{
		Name:                "free",
		DiskLimitBytes:      1 * gib,
		MonthlyTokenLimit:   10_000_000,
		MaxMembersPerOrg:    5,
		MaxOwnedOrgsPerUser: 1,
		MaxProjectsPerOrg:   10,
	}
}
