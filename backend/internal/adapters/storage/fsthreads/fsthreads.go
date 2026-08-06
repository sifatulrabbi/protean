// Package fsthreads implements ports.ThreadStore on the host filesystem, in
// the layout owned by internal/storage/layout. There is no database: a thread
// is a directory, its metadata is one JSON file, and every message is one more.
//
// Three properties hold everywhere in this package:
//
//   - Writes are atomic. Data is staged in a temp file in the destination
//     directory, fsynced, and then linked or renamed into place. A reader never
//     sees a half-written thread.json or message.
//   - Messages are append-only. A message file is created with link(2), so an
//     ID that already exists fails instead of overwriting.
//   - Every write is preflighted against the entitlements engine, and a
//     rejection leaves nothing behind (D17).
package fsthreads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
	"github.com/sifatulrabbi/protean/backend/internal/storage/ulid"
)

const (
	dirMode  = layout.ControlDirMode
	fileMode = layout.FileMode
)

// Options are the store's injected collaborators.
type Options struct {
	// DataDir is the platform data root. Everything is resolved under it by
	// the layout package.
	DataDir string

	// Entitlements is the disk-quota preflight. It is required: a store that
	// could write without asking would be a hole in the quota system.
	Entitlements ports.Entitlements

	// Clock stamps CreatedAt and UpdatedAt and drives ULID generation.
	Clock ports.Clock

	Logger *slog.Logger
}

// Store is the filesystem-backed ports.ThreadStore.
type Store struct {
	dataDir string
	ent     ports.Entitlements
	clock   ports.Clock
	ids     *ulid.Generator
	log     *slog.Logger
	locks   *keyedMutex

	// afterStage is a test seam: it runs after a temp file is written and
	// before it is committed, so the tests can prove that a failure between
	// the two leaves nothing behind. It is nil in production.
	afterStage func(tmpPath string) error
}

var _ ports.ThreadStore = (*Store)(nil)

// New builds a Store. Entitlements and Clock must be non-nil.
func New(opts Options) *Store {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Store{
		dataDir: filepath.Clean(opts.DataDir),
		ent:     opts.Entitlements,
		clock:   opts.Clock,
		ids:     ulid.NewGenerator(opts.Clock),
		log:     logger.With("component", "fsthreads"),
		locks:   newKeyedMutex(),
	}
}

// CreateThread mints a thread ULID and writes its thread.json.
func (s *Store) CreateThread(ctx context.Context, orgID, projectID, title string) (ports.Thread, error) {
	if err := checkProject(orgID, projectID); err != nil {
		return ports.Thread{}, err
	}
	if err := layout.EnsureProject(s.dataDir, orgID, projectID); err != nil {
		return ports.Thread{}, err
	}

	now := s.clock.Now().UTC()
	thread := ports.Thread{
		ID:        s.ids.New(),
		OrgID:     orgID,
		ProjectID: projectID,
		Title:     title,
		CreatedAt: now,
		UpdatedAt: now,
		Tasks:     []ports.Task{},
	}

	unlock := s.locks.lock(s.key(orgID, projectID, thread.ID))
	defer unlock()

	if err := s.writeThread(ctx, thread); err != nil {
		// The thread never existed, so its directory is ours to remove.
		_ = os.RemoveAll(layout.ThreadDir(s.dataDir, orgID, projectID, thread.ID))
		return ports.Thread{}, err
	}
	return thread, nil
}

// GetThread reads one thread.json.
func (s *Store) GetThread(_ context.Context, orgID, projectID, threadID string) (ports.Thread, error) {
	if err := checkThread(orgID, projectID, threadID); err != nil {
		return ports.Thread{}, err
	}
	return s.readThread(orgID, projectID, threadID)
}

// ListThreads returns the project's threads in ULID order. Directory entries
// are already sorted by name, and a ULID sorts by creation time, so the listing
// needs no sort of its own.
func (s *Store) ListThreads(_ context.Context, orgID, projectID string) ([]ports.Thread, error) {
	if err := checkProject(orgID, projectID); err != nil {
		return nil, err
	}

	dir := layout.ThreadsDir(s.dataDir, orgID, projectID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", dir, err)
	}

	threads := make([]ports.Thread, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || !layout.ValidID(e.Name()) {
			continue
		}
		thread, err := s.readThread(orgID, projectID, e.Name())
		if err != nil {
			// One unreadable thread must not hide every other thread of the
			// project from its owner.
			s.log.Warn("skipping unreadable thread",
				"org_id", orgID, "project_id", projectID, "thread_id", e.Name(), "err", err)
			continue
		}
		threads = append(threads, thread)
	}
	return threads, nil
}

// UpdateTasks replaces the task list and bumps UpdatedAt.
func (s *Store) UpdateTasks(ctx context.Context, orgID, projectID, threadID string, tasks []ports.Task) (ports.Thread, error) {
	if err := checkThread(orgID, projectID, threadID); err != nil {
		return ports.Thread{}, err
	}
	if err := validateTasks(tasks); err != nil {
		return ports.Thread{}, err
	}

	unlock := s.locks.lock(s.key(orgID, projectID, threadID))
	defer unlock()

	thread, err := s.readThread(orgID, projectID, threadID)
	if err != nil {
		return ports.Thread{}, err
	}

	thread.Tasks = append([]ports.Task{}, tasks...)
	thread.UpdatedAt = s.clock.Now().UTC()

	if err := s.writeThread(ctx, thread); err != nil {
		return ports.Thread{}, err
	}
	return thread, nil
}

// AppendMessage writes one new message file and returns it.
func (s *Store) AppendMessage(ctx context.Context, orgID, projectID, threadID, role string, content json.RawMessage) (ports.Message, error) {
	if err := checkThread(orgID, projectID, threadID); err != nil {
		return ports.Message{}, err
	}
	if role == "" {
		return ports.Message{}, fmt.Errorf("%w: role is empty", ports.ErrInvalidRole)
	}
	if len(content) == 0 || !json.Valid(content) {
		return ports.Message{}, ports.ErrInvalidContent
	}

	unlock := s.locks.lock(s.key(orgID, projectID, threadID))
	defer unlock()

	if err := s.requireThread(orgID, projectID, threadID); err != nil {
		return ports.Message{}, err
	}

	msg := ports.Message{
		ID:        s.ids.New(),
		Role:      role,
		CreatedAt: s.clock.Now().UTC(),
		Content:   content,
	}
	data, err := marshal(msg)
	if err != nil {
		return ports.Message{}, err
	}

	if err := s.ent.CheckDiskWrite(ctx, orgID, int64(len(data))); err != nil {
		return ports.Message{}, err
	}

	dir := layout.MessagesDir(s.dataDir, orgID, projectID, threadID)
	tmp, err := s.stage(dir, data)
	if err != nil {
		return ports.Message{}, err
	}
	target := layout.MessagePath(s.dataDir, orgID, projectID, threadID, msg.ID)
	if err := commitLink(tmp, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return ports.Message{}, fmt.Errorf("%s: %w", msg.ID, ports.ErrMessageExists)
		}
		return ports.Message{}, fmt.Errorf("commit %q: %w", target, err)
	}
	return msg, nil
}

// ListMessages returns the thread's messages in ULID order.
func (s *Store) ListMessages(_ context.Context, orgID, projectID, threadID string) ([]ports.Message, error) {
	if err := checkThread(orgID, projectID, threadID); err != nil {
		return nil, err
	}
	if err := s.requireThread(orgID, projectID, threadID); err != nil {
		return nil, err
	}

	dir := layout.MessagesDir(s.dataDir, orgID, projectID, threadID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %q: %w", dir, err)
	}

	messages := make([]ports.Message, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		id, ok := layout.MessageIDFromFile(e.Name())
		if !ok {
			continue
		}
		msg, err := s.readMessage(orgID, projectID, threadID, id)
		if err != nil {
			s.log.Warn("skipping unreadable message",
				"org_id", orgID, "project_id", projectID, "thread_id", threadID,
				"message_id", id, "err", err)
			continue
		}
		messages = append(messages, msg)
	}
	return messages, nil
}

// GetMessage reads one message file.
func (s *Store) GetMessage(_ context.Context, orgID, projectID, threadID, messageID string) (ports.Message, error) {
	if err := checkThread(orgID, projectID, threadID); err != nil {
		return ports.Message{}, err
	}
	if err := layout.CheckID("message id", messageID); err != nil {
		return ports.Message{}, err
	}
	return s.readMessage(orgID, projectID, threadID, messageID)
}

// writeThread preflights the quota and then replaces thread.json atomically.
// It assumes the caller holds the thread lock.
func (s *Store) writeThread(ctx context.Context, thread ports.Thread) error {
	data, err := marshal(thread)
	if err != nil {
		return err
	}
	// The delta is the whole serialized size rather than the growth over the
	// previous version: overcounting an update by a few hundred bytes is the
	// safe direction to be wrong in a quota preflight.
	if err := s.ent.CheckDiskWrite(ctx, thread.OrgID, int64(len(data))); err != nil {
		return err
	}

	dir := layout.ThreadDir(s.dataDir, thread.OrgID, thread.ProjectID, thread.ID)
	if err := os.MkdirAll(filepath.Join(dir, layout.MessagesDirName), dirMode); err != nil {
		return fmt.Errorf("create %q: %w", dir, err)
	}
	tmp, err := s.stage(dir, data)
	if err != nil {
		return err
	}
	return commitRename(tmp, layout.ThreadJSONPath(s.dataDir, thread.OrgID, thread.ProjectID, thread.ID))
}

func (s *Store) readThread(orgID, projectID, threadID string) (ports.Thread, error) {
	path := layout.ThreadJSONPath(s.dataDir, orgID, projectID, threadID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ports.Thread{}, fmt.Errorf("%s: %w", threadID, ports.ErrThreadNotFound)
		}
		return ports.Thread{}, fmt.Errorf("read %q: %w", path, err)
	}

	var thread ports.Thread
	if err := json.Unmarshal(data, &thread); err != nil {
		return ports.Thread{}, &ports.CorruptFileError{Path: path, Err: err}
	}
	if thread.ID != threadID {
		return ports.Thread{}, &ports.CorruptFileError{
			Path: path,
			Err:  fmt.Errorf("thread id %q does not match its directory", thread.ID),
		}
	}
	if thread.Tasks == nil {
		thread.Tasks = []ports.Task{}
	}
	return thread, nil
}

func (s *Store) readMessage(orgID, projectID, threadID, messageID string) (ports.Message, error) {
	path := layout.MessagePath(s.dataDir, orgID, projectID, threadID, messageID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ports.Message{}, fmt.Errorf("%s: %w", messageID, ports.ErrMessageNotFound)
		}
		return ports.Message{}, fmt.Errorf("read %q: %w", path, err)
	}

	var msg ports.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return ports.Message{}, &ports.CorruptFileError{Path: path, Err: err}
	}
	if msg.ID != messageID {
		return ports.Message{}, &ports.CorruptFileError{
			Path: path,
			Err:  fmt.Errorf("message id %q does not match its file name", msg.ID),
		}
	}
	return msg, nil
}

// requireThread fails unless the thread exists, without parsing it.
func (s *Store) requireThread(orgID, projectID, threadID string) error {
	path := layout.ThreadJSONPath(s.dataDir, orgID, projectID, threadID)
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", threadID, ports.ErrThreadNotFound)
		}
		return fmt.Errorf("stat %q: %w", path, err)
	}
	return nil
}

func (s *Store) key(orgID, projectID, threadID string) string {
	return orgID + "/" + projectID + "/" + threadID
}

// marshal renders one record. The files are indented because "everything lives
// in the filesystem" only pays off if a human can read what is there.
func marshal(v any) ([]byte, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return append(data, '\n'), nil
}

func checkProject(orgID, projectID string) error {
	if err := layout.CheckID("org id", orgID); err != nil {
		return err
	}
	return layout.CheckID("project id", projectID)
}

func checkThread(orgID, projectID, threadID string) error {
	if err := checkProject(orgID, projectID); err != nil {
		return err
	}
	return layout.CheckID("thread id", threadID)
}

// validateTasks rejects a task list the harness could not render: every task
// needs an identity, a title, and a known status, and no two may share an ID.
func validateTasks(tasks []ports.Task) error {
	seen := make(map[string]struct{}, len(tasks))
	for i, t := range tasks {
		switch {
		case t.ID == "":
			return fmt.Errorf("%w: task %d has no id", ports.ErrInvalidTask, i)
		case t.Title == "":
			return fmt.Errorf("%w: task %q has no title", ports.ErrInvalidTask, t.ID)
		case !t.Status.Valid():
			return fmt.Errorf("%w: task %q has status %q", ports.ErrInvalidTask, t.ID, t.Status)
		}
		if _, dup := seen[t.ID]; dup {
			return fmt.Errorf("%w: duplicate task id %q", ports.ErrInvalidTask, t.ID)
		}
		seen[t.ID] = struct{}{}
	}
	return nil
}
