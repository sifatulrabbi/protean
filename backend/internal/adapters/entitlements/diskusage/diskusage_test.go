package diskusage

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %q: %v", path, err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func TestOrgUsageBytes(t *testing.T) {
	dataDir := t.TempDir()
	fs := New(dataDir)
	ctx := context.Background()

	if got, err := fs.OrgUsageBytes(ctx, "missing-org"); err != nil || got != 0 {
		t.Fatalf("missing org = (%d, %v), want (0, nil)", got, err)
	}

	orgDir := fs.OrgDir("org-1")
	writeFile(t, filepath.Join(orgDir, "a.txt"), 100)
	writeFile(t, filepath.Join(orgDir, "projects", "p1", "thread.json"), 250)
	writeFile(t, filepath.Join(orgDir, "projects", "p1", "memories", "m.md"), 1)
	writeFile(t, filepath.Join(fs.OrgDir("org-2"), "b.bin"), 9_999)

	if err := os.MkdirAll(filepath.Join(orgDir, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir empty: %v", err)
	}

	got, err := fs.OrgUsageBytes(ctx, "org-1")
	if err != nil {
		t.Fatalf("OrgUsageBytes: %v", err)
	}
	if want := int64(351); got != want {
		t.Fatalf("OrgUsageBytes = %d, want %d", got, want)
	}
}

func TestListOrgs(t *testing.T) {
	dataDir := t.TempDir()
	fs := New(dataDir)
	ctx := context.Background()

	orgs, err := fs.ListOrgs(ctx)
	if err != nil || len(orgs) != 0 {
		t.Fatalf("ListOrgs on missing root = (%v, %v), want (empty, nil)", orgs, err)
	}

	writeFile(t, filepath.Join(fs.OrgDir("org-a"), "f"), 1)
	writeFile(t, filepath.Join(fs.OrgDir("org-b"), "f"), 1)
	writeFile(t, filepath.Join(fs.OrgsDir(), "stray-file"), 1)

	orgs, err = fs.ListOrgs(ctx)
	if err != nil {
		t.Fatalf("ListOrgs: %v", err)
	}
	sort.Strings(orgs)
	if len(orgs) != 2 || orgs[0] != "org-a" || orgs[1] != "org-b" {
		t.Fatalf("ListOrgs = %v, want [org-a org-b]", orgs)
	}
}

func TestHostDisk(t *testing.T) {
	fs := New(filepath.Join(t.TempDir(), "not", "created", "yet"))

	free, total, err := fs.HostDisk(context.Background())
	if err != nil {
		t.Fatalf("HostDisk: %v", err)
	}
	if total <= 0 {
		t.Fatalf("total = %d, want a positive size", total)
	}
	if free < 0 || free > total {
		t.Fatalf("free = %d, want between 0 and %d", free, total)
	}
}
