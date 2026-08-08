package fsthreads

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
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
		s.removeTemp(tmp)
		return "", fmt.Errorf("stage %q: %w", tmp, err)
	}
	return tmp, nil
}

// commitRename moves a staged file into place, replacing whatever was there.
// A reader either sees the whole old file or the whole new one.
func (s *Store) commitRename(tmp, target string) error {
	if err := os.Rename(tmp, target); err != nil {
		s.removeTemp(tmp)
		return fmt.Errorf("commit %q: %w", target, err)
	}
	return s.syncDir(filepath.Dir(target))
}

// commitLink is commitRename for a file that must not already exist. link(2)
// fails with EEXIST instead of clobbering, which is how append-only is
// enforced by the filesystem rather than by a check the caller could race.
func (s *Store) commitLink(tmp, target string) error {
	err := os.Link(tmp, target)
	s.removeTemp(tmp)
	if err != nil {
		return err
	}
	return s.syncDir(filepath.Dir(target))
}

func (s *Store) ensureThreadDirs(orgID, projectID, threadID string) (bool, error) {
	threadDir := filepath.Join(layout.ThreadsDir(s.dataDir, orgID, projectID), threadID)
	created, err := mkdir(threadDir, dirMode)
	if err != nil {
		return false, err
	}
	if created {
		if err := s.syncDir(filepath.Dir(threadDir)); err != nil {
			return true, err
		}
	}

	messagesDir := filepath.Join(threadDir, layout.MessagesDirName)
	messagesCreated, err := mkdir(messagesDir, dirMode)
	if err != nil {
		return created, err
	}
	if messagesCreated {
		if err := s.syncDir(threadDir); err != nil {
			return created, err
		}
	}
	return created, nil
}

func mkdir(path string, mode os.FileMode) (bool, error) {
	if err := os.Mkdir(path, mode); err != nil {
		if errors.Is(err, fs.ErrExist) {
			info, statErr := os.Stat(path)
			if statErr != nil {
				return false, fmt.Errorf("stat %q: %w", path, statErr)
			}
			if info.IsDir() {
				return false, nil
			}
		}
		return false, fmt.Errorf("create %q: %w", path, err)
	}
	return true, nil
}

func (s *Store) removeTemp(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.log.Warn("failed to clean temporary file", "path", path, "err", err)
	}
}

func (s *Store) scavengeTemps() {
	root := layout.OrgsRoot(s.dataDir)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() && strings.HasPrefix(entry.Name(), tempPrefix) {
			if err := os.Remove(path); err != nil {
				s.log.Warn("failed to scavenge temporary file", "path", path, "err", err)
			}
		}
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		s.log.Warn("failed to scan for temporary files", "path", root, "err", err)
	}
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
