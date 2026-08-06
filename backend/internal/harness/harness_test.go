package harness

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/adapters/storage/fsthreads"
	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
)

const (
	testOrg     = "org-1"
	testProject = "proj-1"
	testUser    = "user-1"
)

// rig is a harness on a real thread store over a temp data directory, a fake
// provider, and fake entitlements. The store is real on purpose: the
// assertions are about what actually lands on disk.
type rig struct {
	t        *testing.T
	dataDir  string
	threads  *fsthreads.Store
	ent      *fakeEntitlements
	provider *fakeProvider
	tools    *Registry
	harness  *Harness
	thread   ports.Thread

	events []Event
}

func newRig(t *testing.T, provider *fakeProvider, maxTurns int) *rig {
	t.Helper()

	dataDir := t.TempDir()
	clock := newFakeClock(t, "2026-03-15T10:00:00Z")
	ent := newFakeEntitlements()
	threads := fsthreads.New(fsthreads.Options{
		DataDir:      dataDir,
		Entitlements: ent,
		Clock:        clock,
		Logger:       discardLogger(),
	})

	thread, err := threads.CreateThread(context.Background(), testOrg, testProject, "test thread")
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	tools := NewRegistry()
	h, err := New(Deps{
		DataDir:      dataDir,
		Provider:     provider,
		Threads:      threads,
		Entitlements: ent,
		Tools:        tools,
		Clock:        clock,
		Model:        "test/model",
		MaxTurns:     maxTurns,
		Logger:       discardLogger(),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return &rig{t: t, dataDir: dataDir, threads: threads, ent: ent, provider: provider, tools: tools, harness: h, thread: thread}
}

func (r *rig) run(ctx context.Context, message string) error {
	r.t.Helper()
	return r.harness.Run(ctx, RunSpec{
		OrgID:       testOrg,
		ProjectID:   testProject,
		ThreadID:    r.thread.ID,
		UserID:      testUser,
		UserMessage: message,
		Emit:        func(ev Event) { r.events = append(r.events, ev) },
	})
}

func (r *rig) messages() []ports.Message {
	r.t.Helper()
	msgs, err := r.threads.ListMessages(context.Background(), testOrg, testProject, r.thread.ID)
	if err != nil {
		r.t.Fatalf("ListMessages: %v", err)
	}
	return msgs
}

// storedShape renders each persisted message as "role:block,block" so a whole
// conversation can be asserted in one comparison.
func (r *rig) storedShape() []string {
	r.t.Helper()
	var out []string
	for _, m := range r.messages() {
		blocks, err := DecodeContent(m.Content)
		if err != nil {
			r.t.Fatalf("decode message %s: %v", m.ID, err)
		}
		kinds := make([]string, 0, len(blocks))
		for _, b := range blocks {
			kinds = append(kinds, string(b.Type))
		}
		out = append(out, m.Role+":"+strings.Join(kinds, ","))
	}
	return out
}

func (r *rig) eventTypes() []EventType {
	types := make([]EventType, 0, len(r.events))
	for _, ev := range r.events {
		types = append(types, ev.Type)
	}
	return types
}

func (r *rig) lastEvent() Event {
	r.t.Helper()
	if len(r.events) == 0 {
		r.t.Fatal("no events were emitted")
	}
	return r.events[len(r.events)-1]
}

func equalStrings(a []string, b ...string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func equalEventTypes(got []EventType, want ...EventType) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func TestRunMultiTurnToolConversation(t *testing.T) {
	provider := newFakeProvider(
		toolTurn("Let me look.", "call_1", "Grep", `{"pattern":"TODO"}`, ports.Usage{InputTokens: 100, OutputTokens: 20}),
		textTurn("Found one TODO.", ports.Usage{InputTokens: 150, OutputTokens: 12}),
	)
	r := newRig(t, provider, 0)

	grep := &echoTool{name: "Grep", result: "main.go:12: TODO"}
	if err := r.tools.Register(grep); err != nil {
		t.Fatalf("Register: %v", err)
	}
	writeFile(t, layout.OrgAgentsMD(r.dataDir, testOrg), "Be terse.")

	if err := r.run(context.Background(), "any TODOs left?"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := r.storedShape(); !equalStrings(got,
		"user:text",
		"assistant:text,tool_call",
		"tool:tool_result",
		"assistant:text",
	) {
		t.Errorf("persisted messages = %v", got)
	}

	if got := r.eventTypes(); !equalEventTypes(got,
		EventMessagePersisted, // user
		EventTurnStarted,
		EventTextDelta,
		EventUsage,
		EventMessagePersisted, // assistant with the tool call
		EventToolCallStarted,
		EventToolCallFinished,
		EventMessagePersisted, // tool result
		EventTurnStarted,
		EventTextDelta,
		EventUsage,
		EventMessagePersisted, // final assistant answer
		EventDone,
	) {
		t.Errorf("events = %v", got)
	}

	if done := r.lastEvent(); done.StopReason != StopEndTurn || done.Turn != 2 {
		t.Errorf("done event = %+v", done)
	}

	// Token accounting: one record per provider call, in order.
	want := []ports.Usage{{InputTokens: 100, OutputTokens: 20}, {InputTokens: 150, OutputTokens: 12}}
	got := r.ent.recordedUsage()
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("recorded usage = %+v, want %+v", got, want)
	}
	if r.ent.checks() != 2 {
		t.Errorf("entitlement checks = %d, want one per provider call", r.ent.checks())
	}

	// The tool saw the arguments the model produced.
	if calls := grep.invocations(); len(calls) != 1 || string(calls[0]) != `{"pattern":"TODO"}` {
		t.Errorf("tool invocations = %v", calls)
	}

	// The second request carries the whole conversation, read back off disk.
	requests := provider.calls()
	if len(requests) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(requests))
	}
	first := requests[0].Messages
	if len(first) != 2 || first[0].Role != ports.RoleSystem || !strings.Contains(first[0].Content, "Be terse.") {
		t.Errorf("first request = %+v", first)
	}
	if first[1].Role != ports.RoleUser || first[1].Content != "any TODOs left?" {
		t.Errorf("first request user message = %+v", first[1])
	}
	second := requests[1].Messages
	if len(second) != 4 {
		t.Fatalf("second request has %d messages, want 4: %+v", len(second), second)
	}
	if second[2].Role != ports.RoleAssistant || len(second[2].ToolCalls) != 1 || second[2].ToolCalls[0].ID != "call_1" {
		t.Errorf("second request assistant message = %+v", second[2])
	}
	if second[3].Role != ports.RoleTool || second[3].ToolCallID != "call_1" || second[3].Content != "main.go:12: TODO" {
		t.Errorf("second request tool message = %+v", second[3])
	}
	if len(requests[0].Tools) != 1 || requests[0].Tools[0].Name != "Grep" {
		t.Errorf("tool definitions = %+v", requests[0].Tools)
	}
}

func TestRunStopsWhenCreditsRunOutMidConversation(t *testing.T) {
	provider := newFakeProvider(
		toolTurn("Working.", "call_1", "Grep", `{}`, ports.Usage{InputTokens: 10, OutputTokens: 5}),
		textTurn("never reached", ports.Usage{}),
	)
	r := newRig(t, provider, 0)
	r.tools.MustRegister(&echoTool{name: "Grep", result: "no matches"})
	r.ent.rejectFrom = 2

	err := r.run(context.Background(), "look around")
	if !errors.Is(err, ports.ErrNoCredits) {
		t.Fatalf("Run = %v, want ErrNoCredits", err)
	}

	// The first turn completed, so its messages stand; nothing of the second
	// turn was written.
	if got := r.storedShape(); !equalStrings(got, "user:text", "assistant:text,tool_call", "tool:tool_result") {
		t.Errorf("persisted messages = %v", got)
	}
	if last := r.lastEvent(); last.Type != EventError || last.Code != ports.ErrNoCredits.Code {
		t.Errorf("last event = %+v, want a no_credits error", last)
	}
	if last := r.lastEvent(); last.Message != ports.ErrNoCredits.Message {
		t.Errorf("error message = %q, want the entitlement engine's own wording", last.Message)
	}
	if calls := provider.calls(); len(calls) != 1 {
		t.Errorf("provider calls = %d, want the second turn to be blocked", len(calls))
	}
}

func TestRunRejectsBeforeWritingAnything(t *testing.T) {
	provider := newFakeProvider(textTurn("never reached", ports.Usage{}))
	r := newRig(t, provider, 0)
	r.ent.rejectFrom = 1

	err := r.run(context.Background(), "hello")
	if !errors.Is(err, ports.ErrNoCredits) {
		t.Fatalf("Run = %v, want ErrNoCredits", err)
	}
	if got := r.messages(); len(got) != 0 {
		t.Errorf("persisted %d messages, want none", len(got))
	}
	if got := r.eventTypes(); !equalEventTypes(got, EventError) {
		t.Errorf("events = %v, want only the error", got)
	}
	if len(provider.calls()) != 0 {
		t.Error("the provider was called although the org has no credits")
	}
}

func TestRunStopsAtMaxTurns(t *testing.T) {
	provider := newFakeProvider(toolTurn("", "call_x", "Loop", `{}`, ports.Usage{InputTokens: 1, OutputTokens: 1}))
	provider.repeat = true

	r := newRig(t, provider, 2)
	r.tools.MustRegister(&echoTool{name: "Loop", result: "again"})

	if err := r.run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(provider.calls()) != 2 {
		t.Errorf("provider calls = %d, want the turn budget to cap them at 2", len(provider.calls()))
	}
	// The tool results of the last turn are persisted, so the thread stays a
	// conversation a later run can continue.
	if got := r.storedShape(); !equalStrings(got,
		"user:text",
		"assistant:tool_call",
		"tool:tool_result",
		"assistant:tool_call",
		"tool:tool_result",
	) {
		t.Errorf("persisted messages = %v", got)
	}
	if done := r.lastEvent(); done.Type != EventDone || done.StopReason != StopMaxTurns {
		t.Errorf("last event = %+v, want done/max_turns", done)
	}
}

func TestRunTurnsToolFailuresIntoResults(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		register bool
		toolErr  error
		want     string
	}{
		{"tool returns an error", "Grep", true, errors.New("ripgrep exited 2"), ToolErrorPrefix + "ripgrep exited 2"},
		{"tool does not exist", "Imaginary", false, nil, ToolErrorPrefix + `there is no tool named "Imaginary"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			provider := newFakeProvider(
				toolTurn("", "call_1", tc.toolName, `{}`, ports.Usage{InputTokens: 1, OutputTokens: 1}),
				textTurn("I could not do that.", ports.Usage{InputTokens: 2, OutputTokens: 2}),
			)
			r := newRig(t, provider, 0)
			if tc.register {
				r.tools.MustRegister(&echoTool{name: tc.toolName, err: tc.toolErr})
			}

			if err := r.run(context.Background(), "try it"); err != nil {
				t.Fatalf("Run: %v", err)
			}

			// The loop survived the failure and produced a final answer.
			if got := r.storedShape(); !equalStrings(got,
				"user:text", "assistant:tool_call", "tool:tool_result", "assistant:text",
			) {
				t.Fatalf("persisted messages = %v", got)
			}

			blocks, err := DecodeContent(r.messages()[2].Content)
			if err != nil {
				t.Fatalf("decode tool result: %v", err)
			}
			if !blocks[0].IsError {
				t.Error("tool result is not marked as an error")
			}
			if !strings.HasPrefix(blocks[0].Result, tc.want) {
				t.Errorf("tool result = %q, want it to start with %q", blocks[0].Result, tc.want)
			}

			var finished *Event
			for i := range r.events {
				if r.events[i].Type == EventToolCallFinished {
					finished = &r.events[i]
				}
			}
			if finished == nil || !finished.IsError {
				t.Errorf("tool_call_finished event = %+v, want is_error", finished)
			}
		})
	}
}

func TestRunCancellationMidStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after the first text delta has been forwarded.
	provider := newFakeProvider(scriptedTurn{
		events: []ports.StreamEvent{
			{Kind: ports.StreamText, Text: "thinking"},
			{Kind: ports.StreamText, Text: " some more"},
			{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 7, OutputTokens: 3}},
		},
		hook: func(i int) {
			if i == 1 {
				cancel()
			}
		},
	})
	defer cancel()

	r := newRig(t, provider, 0)
	err := r.run(ctx, "start")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run = %v, want context.Canceled", err)
	}

	// The user message is on disk; the half-streamed assistant turn is not.
	if got := r.storedShape(); !equalStrings(got, "user:text") {
		t.Errorf("persisted messages = %v", got)
	}
	if last := r.lastEvent(); last.Type != EventError || last.Code != CodeCanceled {
		t.Errorf("last event = %+v, want a canceled error", last)
	}
}

func TestRunReportsProviderFailure(t *testing.T) {
	boom := errors.New("upstream is down")
	r := newRig(t, newFakeProvider(scriptedTurn{callErr: boom}), 0)

	err := r.run(context.Background(), "hello")
	if !errors.Is(err, boom) {
		t.Fatalf("Run = %v, want the provider failure", err)
	}
	if last := r.lastEvent(); last.Type != EventError || last.Code != CodeProviderError {
		t.Errorf("last event = %+v, want a provider error", last)
	}
	if got := r.storedShape(); !equalStrings(got, "user:text") {
		t.Errorf("persisted messages = %v", got)
	}
}

func TestRunMetersUsageThatArrivedBeforeAStreamFailure(t *testing.T) {
	// The provider counted the tokens and then the connection died, so there
	// is no terminating event — exactly what the adapter produces.
	r := newRig(t, newFakeProvider(scriptedTurn{
		events: []ports.StreamEvent{{Kind: ports.StreamText, Text: "partial"}},
		usage:  ports.Usage{InputTokens: 40, OutputTokens: 9},
		err:    errors.New("stream broke"),
	}), 0)

	if err := r.run(context.Background(), "hi"); err == nil {
		t.Fatal("Run = nil, want the stream failure")
	}
	got := r.ent.recordedUsage()
	if len(got) != 1 || got[0] != (ports.Usage{InputTokens: 40, OutputTokens: 9}) {
		t.Errorf("recorded usage = %+v, want the tokens the provider reported", got)
	}
}

func TestRunTruncatesEventPreviews(t *testing.T) {
	long := strings.Repeat("x", previewLimit*2)
	provider := newFakeProvider(
		toolTurn("", "call_1", "Big", `{"blob":"`+long+`"}`, ports.Usage{InputTokens: 1, OutputTokens: 1}),
		textTurn("done", ports.Usage{InputTokens: 1, OutputTokens: 1}),
	)
	r := newRig(t, provider, 0)
	r.tools.MustRegister(&echoTool{name: "Big", result: long})

	if err := r.run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	for _, ev := range r.events {
		switch ev.Type {
		case EventToolCallStarted:
			if !strings.HasSuffix(ev.ArgsPreview, "... (truncated)") {
				t.Errorf("args preview was not truncated: %d bytes", len(ev.ArgsPreview))
			}
		case EventToolCallFinished:
			if !strings.HasSuffix(ev.ResultPreview, "... (truncated)") {
				t.Errorf("result preview was not truncated: %d bytes", len(ev.ResultPreview))
			}
		}
	}

	// The thread keeps the whole result; only the event stream is cut.
	blocks, err := DecodeContent(r.messages()[2].Content)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if blocks[0].Result != long {
		t.Error("the persisted tool result was truncated, it must not be")
	}
}

func TestRunSubstitutesEmptyToolOutput(t *testing.T) {
	provider := newFakeProvider(
		toolTurn("", "call_1", "Quiet", `{}`, ports.Usage{InputTokens: 1, OutputTokens: 1}),
		textTurn("ok", ports.Usage{InputTokens: 1, OutputTokens: 1}),
	)
	r := newRig(t, provider, 0)
	r.tools.MustRegister(&echoTool{name: "Quiet", result: ""})

	if err := r.run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	blocks, err := DecodeContent(r.messages()[2].Content)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if blocks[0].Result != emptyToolResult {
		t.Errorf("tool result = %q, want %q", blocks[0].Result, emptyToolResult)
	}
}

func TestRunRepairsAThreadLeftWithADanglingToolCall(t *testing.T) {
	r := newRig(t, newFakeProvider(textTurn("carrying on", ports.Usage{InputTokens: 1, OutputTokens: 1})), 0)

	// A previous run died between the assistant message and the tool result.
	// Messages are append-only, so the gap is permanent on disk.
	stranded, err := AssistantContent("calling out", []ports.ToolCall{
		{ID: "call_lost", Name: "Grep", Arguments: json.RawMessage(`{}`)},
	})
	if err != nil {
		t.Fatalf("AssistantContent: %v", err)
	}
	if _, err := r.threads.AppendMessage(context.Background(), testOrg, testProject, r.thread.ID, ports.RoleAssistant, stranded); err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}

	if err := r.run(context.Background(), "still there?"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sent := r.provider.calls()[0].Messages
	var answered bool
	for i, m := range sent {
		if m.Role != ports.RoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		if i+1 >= len(sent) || sent[i+1].Role != ports.RoleTool || sent[i+1].ToolCallID != "call_lost" {
			t.Fatalf("the dangling call was sent unanswered: %+v", sent)
		}
		if sent[i+1].Content != UnansweredToolCallResult {
			t.Errorf("synthetic result = %q", sent[i+1].Content)
		}
		answered = true
	}
	if !answered {
		t.Fatalf("the assistant message never reached the provider: %+v", sent)
	}
}

func TestRunRebuildsTheSystemPromptEveryTurn(t *testing.T) {
	provider := newFakeProvider(
		toolTurn("", "call_1", "Plan", `{}`, ports.Usage{InputTokens: 1, OutputTokens: 1}),
		textTurn("done", ports.Usage{InputTokens: 1, OutputTokens: 1}),
	)
	r := newRig(t, provider, 0)

	// A tool that rewrites the task list, the way S6's TaskManage will. Its
	// effect has to be visible to the very next invocation.
	r.tools.MustRegister(NewFuncTool(
		ports.ToolDef{Name: "Plan", Description: "plans", Parameters: json.RawMessage(`{"type":"object"}`)},
		func(ctx context.Context, _ json.RawMessage) (string, error) {
			_, err := r.threads.UpdateTasks(ctx, testOrg, testProject, r.thread.ID, []ports.Task{
				{ID: "t1", Title: "Ship the harness", Status: ports.TaskInProgress},
			})
			return "task added", err
		},
	))

	if err := r.run(context.Background(), "make a plan"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	calls := r.provider.calls()
	if len(calls) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(calls))
	}
	if first := calls[0].Messages; len(first) > 0 && first[0].Role == ports.RoleSystem &&
		strings.Contains(first[0].Content, "Ship the harness") {
		t.Error("the first turn already saw a task the tool had not created yet")
	}
	second := calls[1].Messages
	if len(second) == 0 || second[0].Role != ports.RoleSystem {
		t.Fatalf("the second turn has no system message: %+v", second)
	}
	if !strings.Contains(second[0].Content, "Ship the harness") {
		t.Errorf("the second turn did not see the new task list:\n%s", second[0].Content)
	}
}

func TestRunGivesEveryToolCallItsOwnID(t *testing.T) {
	// A provider that omits ids, or repeats one, would leave tool results with
	// nothing to answer and make the next request unsendable.
	provider := newFakeProvider(
		scriptedTurn{
			events: []ports.StreamEvent{
				{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{Name: "Echo", Arguments: json.RawMessage(`{}`)}},
				{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{ID: "dup", Name: "Echo", Arguments: json.RawMessage(`{}`)}},
				{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{ID: "dup", Name: "Echo", Arguments: json.RawMessage(`{}`)}},
				{Kind: ports.StreamDone, FinishReason: "tool_calls"},
			},
			usage: ports.Usage{InputTokens: 1, OutputTokens: 1},
		},
		textTurn("done", ports.Usage{InputTokens: 1, OutputTokens: 1}),
	)
	r := newRig(t, provider, 0)
	r.tools.MustRegister(&echoTool{name: "Echo", result: "ok"})

	if err := r.run(context.Background(), "go"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sent := r.provider.calls()[1].Messages
	seen := map[string]bool{}
	var callIDs, resultIDs []string
	for _, m := range sent {
		for _, c := range m.ToolCalls {
			if c.ID == "" || seen[c.ID] {
				t.Errorf("tool call id %q is empty or repeated", c.ID)
			}
			seen[c.ID] = true
			callIDs = append(callIDs, c.ID)
		}
		if m.Role == ports.RoleTool {
			resultIDs = append(resultIDs, m.ToolCallID)
		}
	}
	if len(callIDs) != 3 {
		t.Fatalf("got %d tool calls, want 3", len(callIDs))
	}
	if len(resultIDs) != 3 {
		t.Fatalf("got %d tool results, want one per call: %v", len(resultIDs), resultIDs)
	}
	for i, id := range callIDs {
		if resultIDs[i] != id {
			t.Errorf("result %d answers %q, want %q", i, resultIDs[i], id)
		}
	}
}

func TestRunStopsWhenTheOrgCannotWrite(t *testing.T) {
	r := newRig(t, newFakeProvider(textTurn("hello", ports.Usage{})), 0)
	r.ent.diskWriteErr = ports.ErrOrgReadOnly

	err := r.run(context.Background(), "hi")
	if !errors.Is(err, ports.ErrOrgReadOnly) {
		t.Fatalf("Run = %v, want ErrOrgReadOnly", err)
	}
	if got := r.messages(); len(got) != 0 {
		t.Errorf("persisted %d messages although the org is read-only", len(got))
	}
	if last := r.lastEvent(); last.Type != EventError || last.Code != CodeStorageError {
		t.Errorf("last event = %+v, want a storage error", last)
	}
	if len(r.provider.calls()) != 0 {
		t.Error("the provider was called although the user message could not be saved")
	}
}

func TestRunValidatesItsSpec(t *testing.T) {
	r := newRig(t, newFakeProvider(), 0)

	tests := []struct {
		name string
		spec RunSpec
	}{
		{"no org", RunSpec{ProjectID: testProject, ThreadID: r.thread.ID, UserMessage: "hi"}},
		{"traversal in project", RunSpec{OrgID: testOrg, ProjectID: "../etc", ThreadID: r.thread.ID, UserMessage: "hi"}},
		{"no thread", RunSpec{OrgID: testOrg, ProjectID: testProject, UserMessage: "hi"}},
		{"empty message", RunSpec{OrgID: testOrg, ProjectID: testProject, ThreadID: r.thread.ID}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := r.harness.Run(context.Background(), tc.spec); err == nil {
				t.Fatal("want an error")
			}
		})
	}
	if len(r.messages()) != 0 {
		t.Error("an invalid run wrote to the thread")
	}
}

func TestRunFailsOnAMissingThread(t *testing.T) {
	r := newRig(t, newFakeProvider(), 0)
	err := r.harness.Run(context.Background(), RunSpec{
		OrgID: testOrg, ProjectID: testProject, ThreadID: "01JQNOTATHREAD", UserMessage: "hi",
	})
	if !errors.Is(err, ports.ErrThreadNotFound) {
		t.Fatalf("Run = %v, want ErrThreadNotFound", err)
	}
}

func TestNewRequiresEveryCollaborator(t *testing.T) {
	full := Deps{
		Provider:     newFakeProvider(),
		Threads:      fsthreads.New(fsthreads.Options{DataDir: t.TempDir(), Entitlements: newFakeEntitlements(), Clock: newFakeClock(t, "2026-03-15T10:00:00Z")}),
		Entitlements: newFakeEntitlements(),
		Tools:        NewRegistry(),
		Clock:        newFakeClock(t, "2026-03-15T10:00:00Z"),
	}
	if _, err := New(full); err != nil {
		t.Fatalf("New with every dependency: %v", err)
	}

	for name, strip := range map[string]func(*Deps){
		"provider":     func(d *Deps) { d.Provider = nil },
		"threads":      func(d *Deps) { d.Threads = nil },
		"entitlements": func(d *Deps) { d.Entitlements = nil },
		"tools":        func(d *Deps) { d.Tools = nil },
		"clock":        func(d *Deps) { d.Clock = nil },
	} {
		t.Run("without "+name, func(t *testing.T) {
			deps := full
			strip(&deps)
			if _, err := New(deps); err == nil {
				t.Fatalf("New without %s = nil, want an error", name)
			}
		})
	}
}

func TestDefaultMaxTurnsApplies(t *testing.T) {
	r := newRig(t, newFakeProvider(), 0)
	if r.harness.maxTurns != DefaultMaxTurns {
		t.Errorf("maxTurns = %d, want %d", r.harness.maxTurns, DefaultMaxTurns)
	}
}

func TestEventsAreJSONMarshalable(t *testing.T) {
	usage := ports.Usage{InputTokens: 3, OutputTokens: 4}
	for _, ev := range []Event{
		{Type: EventTurnStarted, ThreadID: "t", Turn: 1},
		{Type: EventTextDelta, ThreadID: "t", Turn: 1, Text: "hi"},
		{Type: EventToolCallStarted, ThreadID: "t", Turn: 1, ToolName: "Grep", ArgsPreview: "{}"},
		{Type: EventUsage, ThreadID: "t", Turn: 1, Usage: &usage},
		{Type: EventDone, ThreadID: "t", Turn: 1, StopReason: StopEndTurn},
		{Type: EventError, ThreadID: "t", Code: CodeCanceled, Message: "cancelled"},
	} {
		data, err := json.Marshal(ev)
		if err != nil {
			t.Fatalf("marshal %s: %v", ev.Type, err)
		}
		var back Event
		if err := json.Unmarshal(data, &back); err != nil {
			t.Fatalf("unmarshal %s: %v", ev.Type, err)
		}
		if back.Type != ev.Type {
			t.Errorf("round trip changed the type: %s -> %s", ev.Type, back.Type)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

func dirOf(path string) string {
	if i := strings.LastIndex(path, "/"); i > 0 {
		return path[:i]
	}
	return "."
}
