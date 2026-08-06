package ports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// TaskStatus is the state of one entry in a thread's task list. The LLM drives
// these through the TaskManage tool; the harness injects the list into context
// on every invocation.
type TaskStatus string

const (
	TaskTodo       TaskStatus = "todo"
	TaskInProgress TaskStatus = "in_progress"
	TaskDone       TaskStatus = "done"
)

// Valid reports whether s is one of the three known statuses.
func (s TaskStatus) Valid() bool {
	switch s {
	case TaskTodo, TaskInProgress, TaskDone:
		return true
	default:
		return false
	}
}

// Task is one entry in a thread's task list. Tasks live inside thread.json,
// not in files of their own: they are rewritten as a set.
type Task struct {
	ID     string     `json:"id"`
	Title  string     `json:"title"`
	Status TaskStatus `json:"status"`
}

// Thread is one conversation. Its metadata and task list live in
// <project>/.protean/threads/<thread-ulid>/thread.json; its messages are one
// file each alongside.
type Thread struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	ProjectID string    `json:"project_id"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Tasks     []Task    `json:"tasks"`
}

// Message is one turn in a thread, stored as one file named by its ULID.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`

	// Content is deliberately opaque here. The harness slice owns the message
	// schema — text, tool calls, thinking blocks — and this port must not have
	// an opinion about it or it would have to change every time the harness
	// learns a new content kind.
	Content json.RawMessage `json:"content"`
}

// ThreadStore persists threads and their messages.
//
// Messages are append-only and immutable: there is no update and no delete, on
// purpose. `.protean/threads` is masked out of every sandbox, so the agent
// cannot reach it; the store is the only writer, and a message file is written
// exactly once. A conversation is therefore an audit log, not a mutable
// document, and replaying a thread always yields what the model actually saw.
//
// Every write path is preflighted against the entitlements engine, because
// thread data counts against the org's disk quota (D2/D15). A rejection leaves
// no partial state on disk (D17).
//
// Implementations must be safe for concurrent use, including concurrent
// AppendMessage calls on the same thread.
type ThreadStore interface {
	// CreateThread mints a ULID, writes thread.json, and returns the thread.
	// The project skeleton is created if it does not exist yet.
	CreateThread(ctx context.Context, orgID, projectID, title string) (Thread, error)

	// GetThread reads one thread's metadata and task list. It fails with
	// ErrThreadNotFound when the thread does not exist and with a
	// *CorruptFileError when thread.json cannot be parsed.
	GetThread(ctx context.Context, orgID, projectID, threadID string) (Thread, error)

	// ListThreads returns every thread of one project in ULID-lexical order,
	// which is creation order. Corrupt entries are skipped and logged rather
	// than failing the whole listing.
	ListThreads(ctx context.Context, orgID, projectID string) ([]Thread, error)

	// UpdateTasks replaces the thread's task list wholesale and bumps
	// UpdatedAt. It fails with ErrInvalidTask when any task is malformed.
	UpdateTasks(ctx context.Context, orgID, projectID, threadID string, tasks []Task) (Thread, error)

	// AppendMessage writes one new message file. The ID is minted here, and
	// creation is exclusive: an existing file for that ID is a hard error
	// (ErrMessageExists), never an overwrite.
	AppendMessage(ctx context.Context, orgID, projectID, threadID, role string, content json.RawMessage) (Message, error)

	// ListMessages returns every message of one thread in ULID-lexical order.
	// Corrupt entries are skipped and logged.
	ListMessages(ctx context.Context, orgID, projectID, threadID string) ([]Message, error)

	// GetMessage reads one message. It fails with ErrMessageNotFound when the
	// message does not exist and with a *CorruptFileError when it cannot be
	// parsed.
	GetMessage(ctx context.Context, orgID, projectID, threadID, messageID string) (Message, error)
}

// Thread store rejections. Match with errors.Is.
var (
	ErrThreadNotFound  = errors.New("thread not found")
	ErrMessageNotFound = errors.New("message not found")
	ErrMessageExists   = errors.New("message already exists")
	ErrInvalidTask     = errors.New("invalid task")
	ErrInvalidRole     = errors.New("invalid message role")
	ErrInvalidContent  = errors.New("message content is not valid JSON")
	ErrCorruptFile     = errors.New("corrupt storage file")
)

// CorruptFileError names the file that could not be parsed, so an operator can
// go and look at it. Match the class with errors.Is(err, ErrCorruptFile).
type CorruptFileError struct {
	Path string
	Err  error
}

func (e *CorruptFileError) Error() string {
	return fmt.Sprintf("corrupt storage file %q: %v", e.Path, e.Err)
}

func (e *CorruptFileError) Unwrap() error { return e.Err }

func (e *CorruptFileError) Is(target error) bool { return target == ErrCorruptFile }
