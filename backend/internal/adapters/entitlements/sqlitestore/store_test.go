package sqlitestore

import (
	"context"
	"errors"
	"math"
	"path/filepath"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "nested", "protean.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestUsageAccumulatesPerOrgAndMonth(t *testing.T) {
	ctx := context.Background()
	store := openTemp(t)

	if in, out, err := store.UsageForMonth(ctx, "org-1", "2026-03"); err != nil || in != 0 || out != 0 {
		t.Fatalf("unknown org = (%d, %d, %v), want (0, 0, nil)", in, out, err)
	}

	adds := []struct {
		orgID, month string
		in, out      int64
	}{
		{"org-1", "2026-03", 100, 10},
		{"org-1", "2026-03", 250, 25},
		{"org-1", "2026-04", 7, 7},
		{"org-2", "2026-03", 1, 2},
	}
	for _, a := range adds {
		if err := store.AddUsage(ctx, a.orgID, a.month, a.in, a.out); err != nil {
			t.Fatalf("AddUsage(%+v): %v", a, err)
		}
	}

	want := []struct {
		orgID, month string
		in, out      int64
	}{
		{"org-1", "2026-03", 350, 35},
		{"org-1", "2026-04", 7, 7},
		{"org-2", "2026-03", 1, 2},
		{"org-2", "2026-04", 0, 0},
	}
	for _, w := range want {
		in, out, err := store.UsageForMonth(ctx, w.orgID, w.month)
		if err != nil {
			t.Fatalf("UsageForMonth(%s, %s): %v", w.orgID, w.month, err)
		}
		if in != w.in || out != w.out {
			t.Errorf("UsageForMonth(%s, %s) = (%d, %d), want (%d, %d)", w.orgID, w.month, in, out, w.in, w.out)
		}
	}
}

func TestOpenIsIdempotentAndPersists(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "protean.db")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := first.AddUsage(ctx, "org-1", "2026-03", 5, 6); err != nil {
		t.Fatalf("AddUsage: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer second.Close()

	in, out, err := second.UsageForMonth(ctx, "org-1", "2026-03")
	if err != nil {
		t.Fatalf("UsageForMonth: %v", err)
	}
	if in != 5 || out != 6 {
		t.Fatalf("UsageForMonth = (%d, %d), want (5, 6)", in, out)
	}
}

func TestAddUsageRejectsNegativeAndNeverAccumulatesBelowZero(t *testing.T) {
	ctx := context.Background()
	store := openTemp(t)

	if err := store.AddUsage(ctx, "org-1", "2026-03", 10, 20); err != nil {
		t.Fatalf("AddUsage: %v", err)
	}
	for _, tc := range []struct {
		name   string
		input  int64
		output int64
	}{
		{name: "negative input", input: -1},
		{name: "negative output", output: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := store.AddUsage(ctx, "org-1", "2026-03", tc.input, tc.output); !errors.Is(err, ports.ErrInvalidTokenUsage) {
				t.Fatalf("AddUsage = %v, want ErrInvalidTokenUsage", err)
			}
		})
	}

	in, out, err := store.UsageForMonth(ctx, "org-1", "2026-03")
	if err != nil {
		t.Fatalf("UsageForMonth: %v", err)
	}
	if in != 10 || out != 20 {
		t.Fatalf("UsageForMonth = (%d, %d), want (10, 20)", in, out)
	}
}

func TestAddUsageSaturatesInsteadOfOverflowingNegative(t *testing.T) {
	ctx := context.Background()
	store := openTemp(t)

	if err := store.AddUsage(ctx, "org-1", "2026-03", math.MaxInt64, math.MaxInt64); err != nil {
		t.Fatalf("first AddUsage: %v", err)
	}
	if err := store.AddUsage(ctx, "org-1", "2026-03", 1, 1); err != nil {
		t.Fatalf("overflow AddUsage: %v", err)
	}

	in, out, err := store.UsageForMonth(ctx, "org-1", "2026-03")
	if err != nil {
		t.Fatalf("UsageForMonth: %v", err)
	}
	if in != math.MaxInt64 || out != math.MaxInt64 {
		t.Fatalf("UsageForMonth = (%d, %d), want saturated MaxInt64", in, out)
	}
}
