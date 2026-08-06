package fsthreads

import (
	"fmt"
	"os"
	"path/filepath"
)

// tempPrefix marks the store's in-flight files. It starts with a dot so a
// listing never mistakes one for a thread or a message.
const tempPrefix = ".tmp-"

// stage writes data to a fresh temp file in dir and fsyncs it, returning the
// temp path. The caller owns the temp file from here: it must either commit it
// or remove it. Nothing else may observe it, because it is never at a name any
// reader looks for.
func (s *Store) stage(dir string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return "", fmt.Errorf("create %q: %w", dir, err)
	}

	f, err := os.CreateTemp(dir, tempPrefix+"*")
	if err != nil {
		return "", fmt.Errorf("stage in %q: %w", dir, err)
	}
	tmp := f.Name()

	err = func() error {
		if _, err := f.Write(data); err != nil {
			return err
		}
		if err := f.Chmod(fileMode); err != nil {
			return err
		}
		return f.Sync()
	}()
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && s.afterStage != nil {
		err = s.afterStage(tmp)
	}
	if err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("stage %q: %w", tmp, err)
	}
	return tmp, nil
}

// commitRename moves a staged file into place, replacing whatever was there.
// A reader either sees the whole old file or the whole new one.
func commitRename(tmp, target string) error {
	if err := os.Rename(tmp, target); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit %q: %w", target, err)
	}
	return syncDir(filepath.Dir(target))
}

// commitLink is commitRename for a file that must not already exist. link(2)
// fails with EEXIST instead of clobbering, which is how append-only is
// enforced by the filesystem rather than by a check the caller could race.
func commitLink(tmp, target string) error {
	err := os.Link(tmp, target)
	_ = os.Remove(tmp)
	if err != nil {
		return err
	}
	return syncDir(filepath.Dir(target))
}

// syncDir flushes a directory entry so a rename or link survives a crash.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("open %q: %w", dir, err)
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", dir, err)
	}
	return nil
}
