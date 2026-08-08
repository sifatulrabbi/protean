package harness

import (
	"unicode/utf8"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// Events are what a run tells the outside world while it happens. They are
// JSON-marshalable because S11 forwards them verbatim over SSE, and they carry
// no pointers into harness state, so an emitter may hold on to one.
//
// Exactly one terminal event ends a run: EventDone when the loop stopped on
// its own terms, EventError when anything rejected it — no credits, a provider
// failure, a cancelled context. Run's return value mirrors that: nil after
// EventDone, non-nil after EventError.
type EventType string

const (
	EventTurnStarted      EventType = "turn_started"
	EventTextDelta        EventType = "text_delta"
	EventToolCallStarted  EventType = "tool_call_started"
	EventToolCallFinished EventType = "tool_call_finished"
	EventMessagePersisted EventType = "message_persisted"
	EventUsage            EventType = "usage"
	EventDone             EventType = "done"
	EventError            EventType = "error"
)

// StopReason says why the loop stopped, on an EventDone.
type StopReason string

const (
	// StopEndTurn is the normal ending: the model answered without asking for
	// another tool.
	StopEndTurn StopReason = "end_turn"

	// StopMaxTurns is the runaway guard: the loop used its whole turn budget.
	StopMaxTurns StopReason = "max_turns"
)

// Error codes carried by EventError. Entitlement rejections reuse the engine's
// own code (for example "no_credits") so the UI can key off one vocabulary.
const (
	CodeCanceled      = "canceled"
	CodeProviderError = "provider_error"
	CodeStorageError  = "storage_error"
	CodeInvalidInput  = "invalid_input"
	CodeInternalError = "internal_error"
)

// Event is one observation from a run.
type Event struct {
	Type     EventType `json:"type"`
	ThreadID string    `json:"thread_id"`

	// Turn is the 1-based provider invocation this event belongs to. It is 0
	// for events raised before the first turn.
	Turn int `json:"turn,omitempty"`

	// MessageID is set on message_persisted and names the stored message.
	MessageID string `json:"message_id,omitempty"`
	Role      string `json:"role,omitempty"`

	// Text is the assistant text fragment, on text_delta.
	Text string `json:"text,omitempty"`

	// Tool call fields, on tool_call_started and tool_call_finished. The
	// previews are truncated: an event stream is for watching, not for
	// replaying, and the full values are in the thread.
	ToolCallID    string `json:"tool_call_id,omitempty"`
	ToolName      string `json:"tool_name,omitempty"`
	ArgsPreview   string `json:"args_preview,omitempty"`
	ResultPreview string `json:"result_preview,omitempty"`
	IsError       bool   `json:"is_error,omitempty"`

	// Usage is the provider-reported token count that was just metered, on
	// usage events.
	Usage *ports.Usage `json:"usage,omitempty"`

	// StopReason is set on done. FinishReason is the provider's own word for
	// why the last turn ended ("stop", "length", "content_filter", ...), which
	// tells a truncated answer from a finished one.
	StopReason   StopReason `json:"stop_reason,omitempty"`
	FinishReason string     `json:"finish_reason,omitempty"`

	// Code and Message are set on error. Message is safe to show to a user.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// previewLimit is how much of a tool's arguments or result an event carries.
const previewLimit = 512

// preview cuts s to previewLimit bytes on a rune boundary and marks the cut.
func preview(s string) string {
	if len(s) <= previewLimit {
		return s
	}
	cut := previewLimit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "... (truncated)"
}
