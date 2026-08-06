package harness

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t *testing.T, rfc3339 string) *fakeClock {
	t.Helper()
	now, err := time.Parse(time.RFC3339, rfc3339)
	if err != nil {
		t.Fatalf("parse time %q: %v", rfc3339, err)
	}
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Millisecond)
	return c.now
}

// scriptedTurn is one provider response the fake will play back.
type scriptedTurn struct {
	// events are yielded in order. A turn that ends cleanly ends with a
	// StreamDone event.
	events []ports.StreamEvent

	// usage is what the provider reported. Like the real adapter's, it is
	// readable whether or not the stream then failed, so a turn that fails
	// after the usage chunk sets this and yields no StreamDone.
	usage ports.Usage

	// err ends the stream early, after the events already yielded.
	err error

	// callErr fails StreamChat itself, before any stream exists.
	callErr error

	// hook runs just before event i is yielded, so a test can cancel the run
	// in the middle of a stream.
	hook func(i int)
}

// textTurn is an assistant turn that only answers.
func textTurn(text string, usage ports.Usage) scriptedTurn {
	return scriptedTurn{
		events: []ports.StreamEvent{
			{Kind: ports.StreamText, Text: text},
			{Kind: ports.StreamDone, Usage: usage, FinishReason: "stop"},
		},
		usage: usage,
	}
}

// toolTurn is an assistant turn that answers and then calls one tool.
func toolTurn(text, callID, tool, args string, usage ports.Usage) scriptedTurn {
	var events []ports.StreamEvent
	if text != "" {
		events = append(events, ports.StreamEvent{Kind: ports.StreamText, Text: text})
	}
	events = append(events,
		ports.StreamEvent{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{
			ID: callID, Name: tool, Arguments: json.RawMessage(args),
		}},
		ports.StreamEvent{Kind: ports.StreamDone, Usage: usage, FinishReason: "tool_calls"},
	)
	return scriptedTurn{events: events, usage: usage}
}

// fakeProvider plays scripted turns back and records what it was asked.
type fakeProvider struct {
	mu       sync.Mutex
	turns    []scriptedTurn
	repeat   bool // replay the last turn forever, for max-turns tests
	requests []ports.ChatRequest
}

var _ ports.LLMProvider = (*fakeProvider)(nil)

func newFakeProvider(turns ...scriptedTurn) *fakeProvider {
	return &fakeProvider{turns: turns}
}

func (p *fakeProvider) StreamChat(ctx context.Context, req ports.ChatRequest) (ports.ChatStream, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.requests)
	p.requests = append(p.requests, req)

	switch {
	case n < len(p.turns):
	case p.repeat && len(p.turns) > 0:
		n = len(p.turns) - 1
	default:
		return nil, errors.New("fakeProvider: unscripted call")
	}

	turn := p.turns[n]
	if turn.callErr != nil {
		return nil, turn.callErr
	}
	return &fakeStream{ctx: ctx, turn: turn}, nil
}

func (p *fakeProvider) calls() []ports.ChatRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]ports.ChatRequest(nil), p.requests...)
}

// fakeStream replays one scripted turn.
//
// It mirrors the real adapter in the two ways the harness depends on: a
// failure suppresses every event still to come, including the terminating one,
// and the usage counter survives that failure. A fake that let a Done event
// through after an error would make the harness's accounting look correct when
// it is not.
type fakeStream struct {
	ctx  context.Context
	turn scriptedTurn
	i    int
	err  error
}

var _ ports.ChatStream = (*fakeStream)(nil)

func (s *fakeStream) Next() (ports.StreamEvent, bool) {
	if s.err != nil {
		return ports.StreamEvent{}, false
	}
	if s.turn.hook != nil {
		s.turn.hook(s.i)
	}
	if err := s.ctx.Err(); err != nil {
		s.err = err
		return ports.StreamEvent{}, false
	}
	if s.i >= len(s.turn.events) {
		s.err = s.turn.err
		return ports.StreamEvent{}, false
	}
	ev := s.turn.events[s.i]
	s.i++
	return ev, true
}

func (s *fakeStream) Usage() ports.Usage { return s.turn.usage }

func (s *fakeStream) Err() error { return s.err }

func (s *fakeStream) Close() error { return nil }

// fakeEntitlements records what the harness metered and can reject from a
// chosen invocation onwards.
type fakeEntitlements struct {
	mu sync.Mutex

	llmChecks    int
	rejectFrom   int // 1-based; 0 never rejects
	rejectErr    error
	usage        []ports.Usage
	recordErr    error
	diskWrites   int
	diskWriteErr error
}

var _ ports.Entitlements = (*fakeEntitlements)(nil)

func newFakeEntitlements() *fakeEntitlements {
	return &fakeEntitlements{rejectErr: ports.ErrNoCredits}
}

func (f *fakeEntitlements) CheckLLMInvocation(context.Context, string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llmChecks++
	if f.rejectFrom > 0 && f.llmChecks >= f.rejectFrom {
		return f.rejectErr
	}
	return nil
}

func (f *fakeEntitlements) RecordTokenUsage(_ context.Context, _ string, in, out int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.recordErr != nil {
		return f.recordErr
	}
	f.usage = append(f.usage, ports.Usage{InputTokens: in, OutputTokens: out})
	return nil
}

func (f *fakeEntitlements) CheckDiskWrite(context.Context, string, int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.diskWrites++
	return f.diskWriteErr
}

func (f *fakeEntitlements) CanAddMember(context.Context, string) error { return nil }

func (f *fakeEntitlements) CanCreateOrg(context.Context, string) error { return nil }

func (f *fakeEntitlements) CanCreateProject(context.Context, string) error { return nil }

func (f *fakeEntitlements) Meters(context.Context, string) (ports.Meters, error) {
	return ports.Meters{}, nil
}

func (f *fakeEntitlements) Subscribe(func(ports.MeterEvent)) {}

func (f *fakeEntitlements) recordedUsage() []ports.Usage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]ports.Usage(nil), f.usage...)
}

func (f *fakeEntitlements) checks() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.llmChecks
}

// echoTool answers with whatever the caller scripted for it.
type echoTool struct {
	name   string
	result string
	err    error
	calls  []json.RawMessage
	mu     sync.Mutex
}

var _ Tool = (*echoTool)(nil)

func (t *echoTool) Name() string { return t.name }

func (t *echoTool) Def() ports.ToolDef {
	return ports.ToolDef{
		Name:        t.name,
		Description: "a test tool",
		Parameters:  json.RawMessage(`{"type":"object"}`),
	}
}

func (t *echoTool) Execute(_ context.Context, args json.RawMessage) (string, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.calls = append(t.calls, args)
	if t.err != nil {
		return "", t.err
	}
	return t.result, nil
}

func (t *echoTool) invocations() []json.RawMessage {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]json.RawMessage(nil), t.calls...)
}
