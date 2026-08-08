package ports

import (
	"context"
	"encoding/json"
)

// Message roles on the provider wire. They are the OpenAI-compatible set,
// which every provider Protean talks to understands.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is one function invocation the model asked for. Arguments is the
// raw JSON object the model produced; it is passed to the tool untouched and
// is not validated here, because only the tool knows its own schema.
type ToolCall struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// ToolDef advertises one tool to the model. Parameters is a JSON Schema object.
type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// ChatMessage is one message in a provider request. It is the transport shape,
// not the storage shape: what lands on disk is the harness's own content
// schema, and the harness converts between the two.
//
// ToolCalls is only set on assistant messages. ToolCallID is only set on tool
// messages and names the call this message answers.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatRequest is one provider invocation. Model is required; the adapter may
// substitute its configured default when it is empty.
type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Tools    []ToolDef     `json:"tools,omitempty"`
}

// Usage is the provider-reported token count for one invocation. It is what
// the entitlements engine meters.
type Usage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
}

// StreamEventKind discriminates a StreamEvent.
type StreamEventKind string

const (
	// StreamText carries a chunk of assistant text in Text.
	StreamText StreamEventKind = "text"

	// StreamToolCall carries one fully accumulated tool call in ToolCall.
	// Providers stream tool-call arguments in fragments; a provider adapter
	// joins them and only emits a call once it is complete.
	StreamToolCall StreamEventKind = "tool_call"

	// StreamDone is the last event of a successful stream. It carries Usage
	// and FinishReason.
	StreamDone StreamEventKind = "done"
)

// StreamEvent is one item of a ChatStream. Only the fields belonging to Kind
// are meaningful.
type StreamEvent struct {
	Kind         StreamEventKind `json:"kind"`
	Text         string          `json:"text,omitempty"`
	ToolCall     ToolCall        `json:"tool_call,omitzero"`
	Usage        Usage           `json:"usage,omitzero"`
	FinishReason string          `json:"finish_reason,omitempty"`
}

// ChatStream is one in-flight streaming completion.
//
// It is a pull iterator rather than a channel on purpose. A channel would
// force every adapter to run a producer goroutine with its own cancellation
// and drain discipline, and a caller that abandoned the stream would leak it;
// with an iterator there is no goroutine at all — Next does the read — so
// abandoning a stream is just Close. It also makes a fake a slice and an
// index.
//
// Next returns false at the end of the stream and on failure; Err then tells
// the two apart. Close releases the underlying resources, is idempotent, and
// is safe to call after the stream is exhausted. Implementations are not safe
// for concurrent use.
type ChatStream interface {
	Next() (StreamEvent, bool)

	// Usage is the token count the provider reported, or the zero Usage when
	// it reported none. It is read after Next returns false and is the
	// authority for metering, because it survives a failure: a stream that
	// died after the provider counted the tokens still spent them. The same
	// value appears on the StreamDone event, which only a clean stream
	// produces.
	Usage() Usage

	Err() error
	Close() error
}

// UsageReportingStream is the optional extension implemented by adapters that
// can distinguish a provider-reported zero count from no usage report at all.
// Harnesses use it to settle zero-token reservations without mistaking a
// truncated stream for a free invocation.
type UsageReportingStream interface {
	ChatStream
	UsageReported() bool
}

// LLMProvider is the model port. Implementations must abort the request when
// ctx is cancelled and must never log or return the API key they authenticate
// with (D11).
type LLMProvider interface {
	// StreamChat starts one streaming completion. The returned stream is owned
	// by the caller, which must Close it.
	StreamChat(ctx context.Context, req ChatRequest) (ChatStream, error)
}
