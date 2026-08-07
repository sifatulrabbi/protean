// Package diskusage implements entitlements.DiskUsage against the local
// filesystem layout.
package diskusage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/sifatulrabbi/protean/backend/internal/entitlements"
)

// OrgsDirName is the directory under the data dir that holds one subdirectory
// per organization.
const OrgsDirName = "protean-organizations"

type FS struct {
	dataDir string
}

var _ entitlements.DiskUsage = FS{}

func New(dataDir string) FS { return FS{dataDir: filepath.Clean(dataDir)} }

// OrgsDir is the root of all organization trees.
func (f FS) OrgsDir() string { return filepath.Join(f.dataDir, OrgsDirName) }

// OrgDir is one organization's tree.
func (f FS) OrgDir(orgID string) string { return filepath.Join(f.OrgsDir(), orgID) }

// OrgUsageBytes sums the apparent size of every regular file in the org tree.
// A missing tree is zero bytes, not an error: an org with no data yet is
// normal. Entries that disappear mid-walk are skipped for the same reason.
func (f FS) OrgUsageBytes(ctx context.Context, orgID string) (int64, error) {
	root := f.OrgDir(orgID)

	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, fmt.Errorf("walk %q: %w", root, err)
	}
	return total, nil
}

// ListOrgs names the immediate subdirectories of the orgs root.
func (f FS) ListOrgs(context.Context) ([]string, error) {
	entries, err := os.ReadDir(f.OrgsDir())
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", f.OrgsDir(), err)
	}

	orgs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			orgs = append(orgs, e.Name())
		}
	}
	return orgs, nil
}

// HostDisk reports the filesystem that holds the data dir.
func (f FS) HostDisk(context.Context) (freeBytes, totalBytes int64, err error) {
	return hostDisk(f.dataDir)
}
