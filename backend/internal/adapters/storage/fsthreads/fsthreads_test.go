package fsthreads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

const (
	orgID     = "01ORG"
	projectID = "01PROJ"
)

type fixture struct {
	store   *Store
	dataDir string
	clock   *fakeClock
	ent     *fakeEntitlements
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	clk := newFakeClock(t, "2026-08-06T10:00:00Z")
	ent := newFakeEntitlements()
	return fixture{
		store: New(Options{
			DataDir:      dir,
			Entitlements: ent,
			Clock:        clk,
			Logger:       discardLogger(),
		}),
		dataDir: dir,
		clock:   clk,
		ent:     ent,
	}
}

func (f fixture) mustCreate(t *testing.T, title string) ports.Thread {
	t.Helper()
	thread, err := f.store.CreateThread(context.Background(), orgID, projectID, title)
	if err != nil {
		t.Fatalf("CreateThread(%q): %v", title, err)
	}
	return thread
}

func content(text string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"text":%q}`, text))
}

func TestCreateAndGetThreadRoundTrip(t *testing.T) {
	f := newFixture(t)

	created := f.mustCreate(t, "first thread")
	if created.ID == "" || len(created.ID) != 26 {
		t.Fatalf("thread id %q is not a ULID", created.ID)
	}
	if created.OrgID != orgID || created.ProjectID != projectID {
		t.Fatalf("scope lost: %+v", created)
	}
	if !created.CreatedAt.Equal(created.UpdatedAt) {
		t.Errorf("a new thread should have CreatedAt == UpdatedAt, got %v vs %v",
			created.CreatedAt, created.UpdatedAt)
	}
	if len(created.Tasks) != 0 {
		t.Errorf("a new thread should have no tasks, got %v", created.Tasks)
	}

	got, err := f.store.GetThread(context.Background(), orgID, projectID, created.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if got.ID != created.ID || got.Title != created.Title || !got.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, created)
	}

	path := layout.ThreadJSONPath(f.dataDir, orgID, projectID, created.ID)
	if _, err := os.Stat(path); err != nil {
		t.Errorf("thread.json not at the layout path: %v", err)
	}
}

func TestGetThreadNotFound(t *testing.T) {
	f := newFixture(t)

	_, err := f.store.GetThread(context.Background(), orgID, projectID, "01MISSING")
	if !errors.Is(err, ports.ErrThreadNotFound) {
		t.Fatalf("got %v, want ErrThreadNotFound", err)
	}
}

func TestListThreadsIsInULIDOrder(t *testing.T) {
	f := newFixture(t)

	var want []string
	for i := range 12 {
		want = append(want, f.mustCreate(t, fmt.Sprintf("thread %d", i)).ID)
		f.clock.advance(time.Millisecond)
	}

	threads, err := f.store.ListThreads(context.Background(), orgID, projectID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != len(want) {
		t.Fatalf("got %d threads, want %d", len(threads), len(want))
	}
	for i, thread := range threads {
		if thread.ID != want[i] {
			t.Fatalf("position %d: got %q, want %q (creation order broken)", i, thread.ID, want[i])
		}
	}
	if !sort.SliceIsSorted(threads, func(i, j int) bool { return threads[i].ID < threads[j].ID }) {
		t.Error("listing is not in lexical ULID order")
	}
}

func TestListThreadsEmptyProject(t *testing.T) {
	f := newFixture(t)

	threads, err := f.store.ListThreads(context.Background(), orgID, "01EMPTY")
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 0 {
		t.Fatalf("got %d threads, want 0", len(threads))
	}
}

func TestUpdateTasksRoundTrip(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "with tasks")

	f.clock.advance(time.Minute)
	tasks := []ports.Task{
		{ID: "t1", Title: "read the spec", Status: ports.TaskDone},
		{ID: "t2", Title: "write the adapter", Status: ports.TaskInProgress},
		{ID: "t3", Title: "wire it up", Status: ports.TaskTodo},
	}

	updated, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, tasks)
	if err != nil {
		t.Fatalf("UpdateTasks: %v", err)
	}
	if !updated.UpdatedAt.After(thread.UpdatedAt) {
		t.Errorf("UpdatedAt not bumped: %v", updated.UpdatedAt)
	}
	if !updated.CreatedAt.Equal(thread.CreatedAt) {
		t.Errorf("CreatedAt moved: %v want %v", updated.CreatedAt, thread.CreatedAt)
	}

	reread, err := f.store.GetThread(context.Background(), orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(reread.Tasks) != len(tasks) {
		t.Fatalf("got %d tasks, want %d", len(reread.Tasks), len(tasks))
	}
	for i, task := range reread.Tasks {
		if task != tasks[i] {
			t.Errorf("task %d: got %+v, want %+v", i, task, tasks[i])
		}
	}

	// Replacing wholesale means an empty list clears the board.
	cleared, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, nil)
	if err != nil {
		t.Fatalf("UpdateTasks(nil): %v", err)
	}
	if len(cleared.Tasks) != 0 {
		t.Errorf("tasks not cleared: %v", cleared.Tasks)
	}
}

func TestUpdateTasksValidation(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "validation")

	tests := []struct {
		name  string
		tasks []ports.Task
	}{
		{"no id", []ports.Task{{Title: "x", Status: ports.TaskTodo}}},
		{"no title", []ports.Task{{ID: "t1", Status: ports.TaskTodo}}},
		{"empty status", []ports.Task{{ID: "t1", Title: "x"}}},
		{"unknown status", []ports.Task{{ID: "t1", Title: "x", Status: "blocked"}}},
		{"duplicate id", []ports.Task{
			{ID: "t1", Title: "x", Status: ports.TaskTodo},
			{ID: "t1", Title: "y", Status: ports.TaskDone},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, tt.tasks)
			if !errors.Is(err, ports.ErrInvalidTask) {
				t.Fatalf("got %v, want ErrInvalidTask", err)
			}
		})
	}

	// A rejected update must not have touched the stored thread.
	got, err := f.store.GetThread(context.Background(), orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(got.Tasks) != 0 {
		t.Errorf("rejected updates leaked tasks: %v", got.Tasks)
	}
}

func TestUpdateTasksUnknownThread(t *testing.T) {
	f := newFixture(t)

	_, err := f.store.UpdateTasks(context.Background(), orgID, projectID, "01MISSING",
		[]ports.Task{{ID: "t1", Title: "x", Status: ports.TaskTodo}})
	if !errors.Is(err, ports.ErrThreadNotFound) {
		t.Fatalf("got %v, want ErrThreadNotFound", err)
	}
}

func TestAppendAndListMessages(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "chat")

	var want []string
	for i := range 20 {
		msg, err := f.store.AppendMessage(context.Background(), orgID, projectID, thread.ID,
			"user", content(fmt.Sprintf("hello %d", i)))
		if err != nil {
			t.Fatalf("AppendMessage %d: %v", i, err)
		}
		want = append(want, msg.ID)
		f.clock.advance(time.Millisecond)
	}

	messages, err := f.store.ListMessages(context.Background(), orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != len(want) {
		t.Fatalf("got %d messages, want %d", len(messages), len(want))
	}
	for i, msg := range messages {
		if msg.ID != want[i] {
			t.Fatalf("position %d: got %q, want %q", i, msg.ID, want[i])
		}
		if msg.Role != "user" {
			t.Errorf("message %d role %q", i, msg.Role)
		}
		var payload struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(msg.Content, &payload); err != nil {
			t.Fatalf("message %d content: %v", i, err)
		}
		if payload.Text != fmt.Sprintf("hello %d", i) {
			t.Errorf("message %d content %q", i, payload.Text)
		}
	}

	one, err := f.store.GetMessage(context.Background(), orgID, projectID, thread.ID, want[3])
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if one.ID != want[3] {
		t.Errorf("GetMessage returned %q, want %q", one.ID, want[3])
	}
}

func TestAppendMessageRejectsBadInput(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "chat")
	ctx := context.Background()

	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "", content("x")); !errors.Is(err, ports.ErrInvalidRole) {
		t.Errorf("empty role: got %v, want ErrInvalidRole", err)
	}
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", nil); !errors.Is(err, ports.ErrInvalidContent) {
		t.Errorf("nil content: got %v, want ErrInvalidContent", err)
	}
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", json.RawMessage("{oops")); !errors.Is(err, ports.ErrInvalidContent) {
		t.Errorf("bad json: got %v, want ErrInvalidContent", err)
	}
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, "01MISSING", "user", content("x")); !errors.Is(err, ports.ErrThreadNotFound) {
		t.Errorf("missing thread: got %v, want ErrThreadNotFound", err)
	}
}

// The append-only rule is enforced by the filesystem, so the test forces the
// collision the way only a broken generator could: it pins the next ID.
func TestAppendMessageNeverOverwrites(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "chat")
	ctx := context.Background()

	first, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("original"))
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	path := layout.MessagePath(f.dataDir, orgID, projectID, thread.ID, first.ID)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}

	// Replay the same ID against the same directory.
	data, err := marshal(ports.Message{
		ID: first.ID, Role: "assistant", CreatedAt: f.clock.Now(), Content: content("impostor"),
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := layout.MessagesDir(f.dataDir, orgID, projectID, thread.ID)
	tmp, err := f.store.stage(dir, data)
	if err != nil {
		t.Fatalf("stage: %v", err)
	}
	if err := f.store.commitLink(tmp, path); !errors.Is(err, fs.ErrExist) {
		t.Fatalf("re-creating an existing message: got %v, want fs.ErrExist", err)
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read message: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("message file was modified:\nbefore %s\nafter  %s", before, after)
	}
	assertNoTempFiles(t, f.dataDir)
}

func TestMessageFilesAreImmutableAcrossOperations(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "chat")
	ctx := context.Background()

	first, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("keep me"))
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	path := layout.MessagePath(f.dataDir, orgID, projectID, thread.ID, first.ID)
	before := statSnapshot(t, path)

	f.clock.advance(time.Second)
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "assistant", content("second")); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if _, err := f.store.UpdateTasks(ctx, orgID, projectID, thread.ID,
		[]ports.Task{{ID: "t1", Title: "x", Status: ports.TaskTodo}}); err != nil {
		t.Fatalf("UpdateTasks: %v", err)
	}

	if got := statSnapshot(t, path); got != before {
		t.Fatalf("first message changed: got %+v, want %+v", got, before)
	}
}

func TestQuotaRejectionLeavesNoState(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	thread := f.mustCreate(t, "before the wall")
	msg, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("allowed"))
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	f.ent.reject(ports.ErrDiskQuotaExceeded)

	if _, err := f.store.CreateThread(ctx, orgID, projectID, "over quota"); !errors.Is(err, ports.ErrDiskQuotaExceeded) {
		t.Fatalf("CreateThread: got %v, want ErrDiskQuotaExceeded", err)
	}
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("rejected")); !errors.Is(err, ports.ErrDiskQuotaExceeded) {
		t.Fatalf("AppendMessage: got %v, want ErrDiskQuotaExceeded", err)
	}
	if _, err := f.store.UpdateTasks(ctx, orgID, projectID, thread.ID,
		[]ports.Task{{ID: "t1", Title: "x", Status: ports.TaskTodo}}); !errors.Is(err, ports.ErrDiskQuotaExceeded) {
		t.Fatalf("UpdateTasks: got %v, want ErrDiskQuotaExceeded", err)
	}

	f.ent.reject(nil)

	threads, err := f.store.ListThreads(ctx, orgID, projectID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != thread.ID {
		t.Fatalf("rejected create left state behind: %+v", threads)
	}
	if len(threads[0].Tasks) != 0 {
		t.Fatalf("rejected task update committed: %v", threads[0].Tasks)
	}

	messages, err := f.store.ListMessages(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != msg.ID {
		t.Fatalf("rejected append committed: %+v", messages)
	}
	assertNoTempFiles(t, f.dataDir)
}

func TestPreflightSeesSerializedSize(t *testing.T) {
	f := newFixture(t)

	thread := f.mustCreate(t, "sized")
	deltas := f.ent.recorded()
	if len(deltas) != 1 {
		t.Fatalf("got %d preflights, want 1", len(deltas))
	}
	data, err := marshal(thread)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if deltas[0] != int64(len(data)) {
		t.Errorf("preflight delta %d, want %d", deltas[0], len(data))
	}
}

func TestFailureBetweenStageAndCommitLeavesNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	thread := f.mustCreate(t, "ok so far")
	boom := errors.New("injected failure")
	f.store.afterStage = func(string) error { return boom }

	if _, err := f.store.CreateThread(ctx, orgID, projectID, "doomed"); !errors.Is(err, boom) {
		t.Fatalf("CreateThread: got %v, want the injected failure", err)
	}
	if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("doomed")); !errors.Is(err, boom) {
		t.Fatalf("AppendMessage: got %v, want the injected failure", err)
	}
	if _, err := f.store.UpdateTasks(ctx, orgID, projectID, thread.ID,
		[]ports.Task{{ID: "t1", Title: "x", Status: ports.TaskTodo}}); !errors.Is(err, boom) {
		t.Fatalf("UpdateTasks: got %v, want the injected failure", err)
	}

	f.store.afterStage = nil
	assertNoTempFiles(t, f.dataDir)

	threads, err := f.store.ListThreads(ctx, orgID, projectID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != thread.ID || len(threads[0].Tasks) != 0 {
		t.Fatalf("failed writes left state behind: %+v", threads)
	}
	messages, err := f.store.ListMessages(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("failed append committed: %+v", messages)
	}
}

func TestConcurrentAppendsAreExactAndOrdered(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "busy")
	ctx := context.Background()

	const goroutines, each = 12, 25
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		ids  []string
		errs []error
	)
	wg.Add(goroutines)
	for g := range goroutines {
		go func() {
			defer wg.Done()
			for i := range each {
				msg, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID,
					"user", content(fmt.Sprintf("g%d-%d", g, i)))
				mu.Lock()
				if err != nil {
					errs = append(errs, err)
				} else {
					ids = append(ids, msg.ID)
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(errs) > 0 {
		t.Fatalf("%d appends failed, first: %v", len(errs), errs[0])
	}
	if len(ids) != goroutines*each {
		t.Fatalf("got %d ids, want %d", len(ids), goroutines*each)
	}

	messages, err := f.store.ListMessages(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != goroutines*each {
		t.Fatalf("got %d messages on disk, want %d", len(messages), goroutines*each)
	}
	for i := 1; i < len(messages); i++ {
		if messages[i].ID <= messages[i-1].ID {
			t.Fatalf("message %d (%q) does not sort after %q", i, messages[i].ID, messages[i-1].ID)
		}
	}
	assertNoTempFiles(t, f.dataDir)
}

func TestConcurrentSameOrgAppendsAcrossThreadsRespectQuota(t *testing.T) {
	dataDir := t.TempDir()
	clk := newFakeClock(t, "2026-08-06T10:00:00Z")
	ent := newQuotaEntitlements(dataDir)
	store := New(Options{
		DataDir:      dataDir,
		Entitlements: ent,
		Clock:        clk,
		Logger:       discardLogger(),
	})
	ctx := context.Background()
	first, err := store.CreateThread(ctx, orgID, projectID, "first")
	if err != nil {
		t.Fatalf("create first thread: %v", err)
	}
	second, err := store.CreateThread(ctx, orgID, projectID, "second")
	if err != nil {
		t.Fatalf("create second thread: %v", err)
	}

	used, err := diskUsage(layout.OrgDir(dataDir, orgID))
	if err != nil {
		t.Fatalf("measure disk usage: %v", err)
	}
	sample, err := marshal(ports.Message{
		ID:        strings.Repeat("0", 26),
		Role:      "user",
		CreatedAt: clk.Now().UTC(),
		Content:   content("one slot"),
	})
	if err != nil {
		t.Fatalf("marshal sample: %v", err)
	}
	ent.setLimit(used + int64(len(sample)))

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, threadID := range []string{first.ID, second.ID} {
		go func() {
			<-start
			_, err := store.AppendMessage(ctx, orgID, projectID, threadID, "user", content("one slot"))
			results <- err
		}()
	}
	close(start)

	var succeeded, rejected int
	for range 2 {
		err := <-results
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ports.ErrDiskQuotaExceeded):
			rejected++
		default:
			t.Fatalf("append returned unexpected error: %v", err)
		}
	}
	if succeeded != 1 || rejected != 1 {
		t.Fatalf("got %d successful and %d rejected appends, want one each", succeeded, rejected)
	}

	finalUsed, err := diskUsage(layout.OrgDir(dataDir, orgID))
	if err != nil {
		t.Fatalf("measure final disk usage: %v", err)
	}
	if finalUsed > ent.limitBytes() {
		t.Fatalf("concurrent appends used %d bytes, exceeding cap %d", finalUsed, ent.limitBytes())
	}
}

// Appends and task updates race on the same thread; the store must serialize
// them without losing either.
func TestConcurrentAppendsAndTaskUpdates(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "busy")
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range n {
			if _, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content(fmt.Sprint(i))); err != nil {
				t.Errorf("AppendMessage: %v", err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := range n {
			tasks := []ports.Task{{ID: fmt.Sprintf("t%d", i), Title: "work", Status: ports.TaskTodo}}
			if _, err := f.store.UpdateTasks(ctx, orgID, projectID, thread.ID, tasks); err != nil {
				t.Errorf("UpdateTasks: %v", err)
				return
			}
		}
	}()
	wg.Wait()

	messages, err := f.store.ListMessages(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != n {
		t.Fatalf("got %d messages, want %d", len(messages), n)
	}
	got, err := f.store.GetThread(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("GetThread: %v", err)
	}
	if len(got.Tasks) != 1 {
		t.Fatalf("thread.json torn or lost: %+v", got.Tasks)
	}
}

func TestCorruptThreadJSON(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	good := f.mustCreate(t, "good")
	f.clock.advance(time.Millisecond)
	bad := f.mustCreate(t, "bad")
	badPath := layout.ThreadJSONPath(f.dataDir, orgID, projectID, bad.ID)
	if err := os.WriteFile(badPath, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	_, err := f.store.GetThread(ctx, orgID, projectID, bad.ID)
	if !errors.Is(err, ports.ErrCorruptFile) {
		t.Fatalf("GetThread: got %v, want ErrCorruptFile", err)
	}
	var corrupt *ports.CorruptFileError
	if !errors.As(err, &corrupt) {
		t.Fatalf("GetThread: got %v, want *CorruptFileError", err)
	}
	if corrupt.Path != badPath {
		t.Errorf("error names %q, want %q", corrupt.Path, badPath)
	}

	threads, err := f.store.ListThreads(ctx, orgID, projectID)
	if err != nil {
		t.Fatalf("ListThreads: %v", err)
	}
	if len(threads) != 1 || threads[0].ID != good.ID {
		t.Fatalf("corrupt thread did not get skipped: %+v", threads)
	}
}

func TestCorruptMessageFile(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	thread := f.mustCreate(t, "chat")

	good, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("fine"))
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	f.clock.advance(time.Millisecond)
	bad, err := f.store.AppendMessage(ctx, orgID, projectID, thread.ID, "user", content("will break"))
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	badPath := layout.MessagePath(f.dataDir, orgID, projectID, thread.ID, bad.ID)
	if err := os.WriteFile(badPath, []byte("garbage"), 0o644); err != nil {
		t.Fatalf("corrupt file: %v", err)
	}

	_, err = f.store.GetMessage(ctx, orgID, projectID, thread.ID, bad.ID)
	var corrupt *ports.CorruptFileError
	if !errors.As(err, &corrupt) || corrupt.Path != badPath {
		t.Fatalf("GetMessage: got %v, want a *CorruptFileError naming %q", err, badPath)
	}

	messages, err := f.store.ListMessages(ctx, orgID, projectID, thread.ID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(messages) != 1 || messages[0].ID != good.ID {
		t.Fatalf("corrupt message did not get skipped: %+v", messages)
	}
}

// A thread.json whose id does not match its directory is corruption too: the
// store must not hand back a record that claims to be something else.
func TestMismatchedThreadIDIsCorrupt(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "mislabeled")

	path := layout.ThreadJSONPath(f.dataDir, orgID, projectID, thread.ID)
	thread.ID = "01SOMETHINGELSE"
	data, err := marshal(thread)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err = f.store.GetThread(context.Background(), orgID, projectID, filepath.Base(filepath.Dir(path)))
	if !errors.Is(err, ports.ErrCorruptFile) {
		t.Fatalf("got %v, want ErrCorruptFile", err)
	}
}

func TestMismatchedThreadScopeIsCorruptAndCannotSteerUpdate(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*ports.Thread)
		escapeDir string
	}{
		{
			name:   "org",
			mutate: func(thread *ports.Thread) { thread.OrgID = "01OTHERORG" },
		},
		{
			name:   "project",
			mutate: func(thread *ports.Thread) { thread.ProjectID = "01OTHERPROJECT" },
		},
		{
			name:      "org traversal",
			mutate:    func(thread *ports.Thread) { thread.OrgID = "../escape" },
			escapeDir: "escape",
		},
		{
			name:      "project traversal",
			mutate:    func(thread *ports.Thread) { thread.ProjectID = "../../../escape" },
			escapeDir: "escape",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			thread := f.mustCreate(t, "mislabeled scope")
			path := layout.ThreadJSONPath(f.dataDir, orgID, projectID, thread.ID)
			tt.mutate(&thread)
			data, err := marshal(thread)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if err := os.WriteFile(path, data, fileMode); err != nil {
				t.Fatalf("write: %v", err)
			}

			if _, err := f.store.GetThread(context.Background(), orgID, projectID, thread.ID); !errors.Is(err, ports.ErrCorruptFile) {
				t.Fatalf("GetThread: got %v, want ErrCorruptFile", err)
			}
			_, err = f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID,
				[]ports.Task{{ID: "t1", Title: "must not escape", Status: ports.TaskTodo}})
			if !errors.Is(err, ports.ErrCorruptFile) {
				t.Fatalf("UpdateTasks: got %v, want ErrCorruptFile", err)
			}
			if tt.escapeDir != "" {
				if _, err := os.Stat(filepath.Join(f.dataDir, tt.escapeDir)); !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("traversal destination exists or cannot be checked: %v", err)
				}
			}
		})
	}
}

func TestCreateThreadSyncsNewDirectoryParents(t *testing.T) {
	f := newFixture(t)
	var synced []string
	f.store.syncDir = func(path string) error {
		synced = append(synced, path)
		return nil
	}

	thread := f.mustCreate(t, "durable")
	threadsDir := layout.ThreadsDir(f.dataDir, orgID, projectID)
	threadDir := layout.ThreadDir(f.dataDir, orgID, projectID, thread.ID)
	if len(synced) < 3 {
		t.Fatalf("synced %v, want threads parent, thread parent, and thread.json commit", synced)
	}
	if synced[0] != threadsDir {
		t.Errorf("first sync = %q, want %q after creating thread directory", synced[0], threadsDir)
	}
	if synced[1] != threadDir {
		t.Errorf("second sync = %q, want %q after creating messages directory", synced[1], threadDir)
	}
}

func TestUpdateTasksPreflightsNetGrowth(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "delta")
	path := layout.ThreadJSONPath(f.dataDir, orgID, projectID, thread.ID)
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat before: %v", err)
	}

	f.clock.advance(time.Second)
	tasks := []ports.Task{{ID: "t1", Title: strings.Repeat("grow", 20), Status: ports.TaskTodo}}
	updated, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, tasks)
	if err != nil {
		t.Fatalf("UpdateTasks: %v", err)
	}
	data, err := marshal(updated)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	deltas := f.ent.recorded()
	want := int64(len(data)) - before.Size()
	if got := deltas[len(deltas)-1]; got != want {
		t.Fatalf("update preflight delta = %d, want net growth %d", got, want)
	}
}

func TestUpdateTasksCanShrinkAtQuota(t *testing.T) {
	f := newFixture(t)
	thread := f.mustCreate(t, "shrink")
	tasks := []ports.Task{{ID: "t1", Title: strings.Repeat("large", 100), Status: ports.TaskTodo}}
	if _, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, tasks); err != nil {
		t.Fatalf("grow tasks: %v", err)
	}
	checksBefore := len(f.ent.recorded())
	f.ent.reject(ports.ErrOrgReadOnly)

	updated, err := f.store.UpdateTasks(context.Background(), orgID, projectID, thread.ID, nil)
	if err != nil {
		t.Fatalf("shrink at quota: %v", err)
	}
	if len(updated.Tasks) != 0 {
		t.Fatalf("tasks not cleared: %v", updated.Tasks)
	}
	if got := len(f.ent.recorded()); got != checksBefore {
		t.Fatalf("shrink performed a quota check: got %d checks, want %d", got, checksBefore)
	}
}

func TestRejectedCreateLeavesNoSkeleton(t *testing.T) {
	f := newFixture(t)
	f.ent.reject(ports.ErrDiskQuotaExceeded)

	if _, err := f.store.CreateThread(context.Background(), orgID, projectID, "rejected"); !errors.Is(err, ports.ErrDiskQuotaExceeded) {
		t.Fatalf("CreateThread: got %v, want ErrDiskQuotaExceeded", err)
	}
	if _, err := os.Stat(layout.OrgsRoot(f.dataDir)); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("rejected create left a storage skeleton: %v", err)
	}
}

func TestNewScavengesOrphanedTempFiles(t *testing.T) {
	dataDir := t.TempDir()
	dir := filepath.Join(layout.OrgsRoot(dataDir), orgID, "orphaned")
	if err := os.MkdirAll(dir, dirMode); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	tmp := filepath.Join(dir, tempPrefix+"crash")
	keep := filepath.Join(dir, "keep.json")
	if err := os.WriteFile(tmp, []byte("partial"), fileMode); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	if err := os.WriteFile(keep, []byte("committed"), fileMode); err != nil {
		t.Fatalf("write committed: %v", err)
	}

	_ = New(Options{
		DataDir:      dataDir,
		Entitlements: newFakeEntitlements(),
		Clock:        newFakeClock(t, "2026-08-06T10:00:00Z"),
		Logger:       discardLogger(),
	})
	if _, err := os.Stat(tmp); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("orphaned temp still exists: %v", err)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("committed file was scavenged: %v", err)
	}
}

func TestIDValidationRejectsTraversal(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	calls := map[string]func() error{
		"create with bad org":     func() error { _, err := f.store.CreateThread(ctx, "../escape", projectID, "x"); return err },
		"create with bad project": func() error { _, err := f.store.CreateThread(ctx, orgID, "..", "x"); return err },
		"get with bad thread":     func() error { _, err := f.store.GetThread(ctx, orgID, projectID, "a/b"); return err },
		"append with bad thread": func() error {
			_, err := f.store.AppendMessage(ctx, orgID, projectID, "../x", "user", content("x"))
			return err
		},
		"get message with bad id": func() error {
			_, err := f.store.GetMessage(ctx, orgID, projectID, "01T", "../../etc/passwd")
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, layout.ErrInvalidID) {
				t.Fatalf("got %v, want ErrInvalidID", err)
			}
		})
	}
}

type fileSnapshot struct {
	size    int64
	modTime time.Time
	content string
}

func statSnapshot(t *testing.T, path string) fileSnapshot {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return fileSnapshot{size: info.Size(), modTime: info.ModTime(), content: string(data)}
}

// assertNoTempFiles proves that every staged write was either committed or
// removed: a leftover temp file is a leak of partial state.
func assertNoTempFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), tempPrefix) {
			t.Errorf("leftover temp file %q", path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %q: %v", root, err)
	}
}

type quotaEntitlements struct {
	*fakeEntitlements
	root  string
	locks *keyedMutex

	mu    sync.Mutex
	limit int64
}

var _ diskWriteReserver = (*quotaEntitlements)(nil)

func newQuotaEntitlements(dataDir string) *quotaEntitlements {
	return &quotaEntitlements{
		fakeEntitlements: newFakeEntitlements(),
		root:             dataDir,
		locks:            newKeyedMutex(),
		limit:            int64(^uint64(0) >> 1),
	}
}

func (q *quotaEntitlements) setLimit(limit int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.limit = limit
}

func (q *quotaEntitlements) limitBytes() int64 {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.limit
}

func (q *quotaEntitlements) CheckDiskWrite(_ context.Context, orgID string, deltaBytes int64) error {
	used, err := diskUsage(layout.OrgDir(q.root, orgID))
	if err != nil {
		return err
	}
	if deltaBytes > q.limitBytes()-used {
		return ports.ErrDiskQuotaExceeded
	}
	return nil
}

func (q *quotaEntitlements) ReserveDiskWrite(ctx context.Context, orgID string, deltaBytes int64) (ports.StructuralAdmission, error) {
	release := q.locks.lock(orgID)
	if err := q.CheckDiskWrite(ctx, orgID, deltaBytes); err != nil {
		release()
		return nil, err
	}
	return releaseFunc(release), nil
}

func diskUsage(root string) (int64, error) {
	var total int64
	err := filepath.WalkDir(root, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}
