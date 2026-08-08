// Package harness is Protean's agentic loop: it assembles the model's
// context, invokes the provider, dispatches the tools the model asks for, and
// persists every turn into the thread.
//
// The harness is deliberately bare-bones. It ships no tools and no persona;
// what the agent can do comes from the registry the boot wiring fills (S6) and
// what it should do comes from the org's AGENTS.md, memories, and skills. This
// package owns two contracts the rest of the system reads: the stored message
// content schema (content.go) and the run event stream (events.go).
//
// It runs on the host, never inside a sandbox: it holds the provider key, and
// only tool execution crosses the boundary (D11).
package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
	"github.com/sifatulrabbi/protean/backend/internal/storage/layout"
	"github.com/sifatulrabbi/protean/backend/internal/storage/ulid"
)

// DefaultMaxTurns bounds one Run when Deps does not say otherwise. It is the
// runaway guard, not a budget: the entitlements engine owns the budget.
const DefaultMaxTurns = 40

const (
	usageRecordAttempts  = 3
	pendingUsageFileName = "pending-llm-usage.jsonl"
)

// ToolErrorPrefix marks a tool result that reports a failure. The model reads
// it, so it is words, not a code — and the loop keeps going, because a tool
// that failed is information, not the end of a conversation.
const ToolErrorPrefix = "Tool execution failed: "

// emptyToolResult stands in for a tool that returned nothing, so no message is
// ever persisted with an empty body.
const emptyToolResult = "(no output)"

// Deps are the harness's injected collaborators.
type Deps struct {
	// DataDir is the platform data root, used for context assembly.
	DataDir string

	Provider     ports.LLMProvider
	Threads      ports.ThreadStore
	Entitlements ports.Entitlements
	Tools        *Registry
	Clock        ports.Clock

	// Model is passed to the provider on every call. Empty leaves the choice
	// to the provider adapter's own default.
	Model string

	// MaxTurns bounds the provider calls of one Run. Zero uses DefaultMaxTurns.
	MaxTurns int

	Logger *slog.Logger
}

// Harness runs conversations. It is safe for concurrent use: it holds no
// per-run state.
type Harness struct {
	provider ports.LLMProvider
	threads  ports.ThreadStore
	ent      ports.Entitlements
	tools    *Registry
	ctxb     *ContextBuilder
	ids      *ulid.Generator
	model    string
	maxTurns int
	log      *slog.Logger
	usageMu  sync.Mutex
}

// New builds a Harness. Every collaborator is required: a harness missing one
// could only fail at the worst possible moment, mid-conversation.
func New(deps Deps) (*Harness, error) {
	switch {
	case deps.Provider == nil:
		return nil, errors.New("harness: provider is required")
	case deps.Threads == nil:
		return nil, errors.New("harness: thread store is required")
	case deps.Entitlements == nil:
		return nil, errors.New("harness: entitlements are required")
	case deps.Tools == nil:
		return nil, errors.New("harness: tool registry is required")
	case deps.Clock == nil:
		return nil, errors.New("harness: clock is required")
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}
	maxTurns := deps.MaxTurns
	if maxTurns <= 0 {
		maxTurns = DefaultMaxTurns
	}
	logger = logger.With("component", "harness")

	return &Harness{
		provider: deps.Provider,
		threads:  deps.Threads,
		ent:      deps.Entitlements,
		tools:    deps.Tools,
		ctxb:     NewContextBuilder(deps.DataDir, logger),
		ids:      ulid.NewGenerator(deps.Clock),
		model:    deps.Model,
		maxTurns: maxTurns,
		log:      logger,
	}, nil
}

// RunSpec is one invocation of the agent on one thread.
type RunSpec struct {
	OrgID     string
	ProjectID string
	ThreadID  string

	// UserID scopes the user memory loaded into context.
	UserID string

	// UserMessage is the turn the user just sent.
	UserMessage string

	// Emit receives the run's events. It is called synchronously from Run's
	// goroutine, in order, and must not block: an SSE writer wraps it, a test
	// appends to a slice. Nil discards the events.
	//
	// It is a function rather than a channel on purpose: a channel would need
	// a consumer for the whole run or the harness would block, and abandoning
	// it would leak. A function has no lifetime to manage.
	Emit func(Event)
}

// Run drives one conversation to a stop.
//
// The loop is: preflight the org's token balance, persist the user message,
// then per turn stream an assistant response, meter its tokens, persist it,
// execute whatever tools it asked for, and go again. It stops when the model
// answers without calling a tool, when the turn budget runs out, when the
// context is cancelled, or when the entitlements engine says no.
//
// Every provider call is preceded by a reservation when the entitlements
// implementation supports it, or by CheckLLMInvocation otherwise. Reported
// usage settles that reservation, with the check/record pair retained as the
// backward-compatible fallback.
func (h *Harness) Run(ctx context.Context, spec RunSpec) error {
	emit := spec.Emit
	if emit == nil {
		emit = func(Event) {}
	}
	fail := func(turn int, code, message string, err error) error {
		emit(Event{Type: EventError, ThreadID: spec.ThreadID, Turn: turn, Code: code, Message: message})
		h.log.Warn("run failed",
			"org_id", spec.OrgID, "project_id", spec.ProjectID, "thread_id", spec.ThreadID,
			"turn", turn, "code", code, "err", err)
		return err
	}

	if err := validateSpec(spec); err != nil {
		return fail(0, CodeInvalidInput, err.Error(), err)
	}

	// Fail on a thread that does not exist before anything else happens, so an
	// addressing mistake is reported as one.
	if _, err := h.threads.GetThread(ctx, spec.OrgID, spec.ProjectID, spec.ThreadID); err != nil {
		return fail(0, CodeStorageError, "This thread could not be opened.", err)
	}

	// The balance is checked before anything is written, so an org with no
	// credits does not even spend disk on a message it cannot answer.
	reservation, err := h.reserveInvocation(ctx, spec.OrgID)
	if err != nil {
		return fail(0, entitlementCode(err), entitlementMessage(err), err)
	}

	if _, err := h.persist(ctx, spec, emit, 0, ports.RoleUser, mustText(spec.UserMessage)); err != nil {
		releaseReservation(reservation)
		return fail(0, CodeStorageError, "Your message could not be saved.", err)
	}

	conv, err := h.history(ctx, spec)
	if err != nil {
		releaseReservation(reservation)
		return fail(0, CodeStorageError, "This thread's history could not be read.", err)
	}

	for turn := 1; turn <= h.maxTurns; turn++ {
		if err := ctx.Err(); err != nil {
			releaseReservation(reservation)
			return fail(turn, CodeCanceled, "The run was cancelled.", err)
		}
		if turn > 1 {
			reservation, err = h.reserveInvocation(ctx, spec.OrgID)
			if err != nil {
				return fail(turn, entitlementCode(err), entitlementMessage(err), err)
			}
		}

		// The system prompt is rebuilt every turn, not once per run: a tool
		// may have edited the task list, a memory, or AGENTS.md since the last
		// one, and the next invocation has to see it (S6's TaskManage depends
		// on this).
		system, err := h.systemPrompt(ctx, spec)
		if err != nil {
			releaseReservation(reservation)
			return fail(turn, CodeStorageError, "This thread could not be opened.", err)
		}

		emit(Event{Type: EventTurnStarted, ThreadID: spec.ThreadID, Turn: turn})

		result, err := h.invoke(ctx, spec, emit, turn, system, conv, reservation)
		reservation = nil // invoke always settles or releases its reservation.
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return fail(turn, CodeCanceled, "The run was cancelled.", ctxErr)
			}
			return fail(turn, CodeProviderError, "The AI provider could not complete this turn.", err)
		}

		if result.text == "" && len(result.calls) == 0 {
			// The provider said nothing at all. There is nothing to persist
			// and nothing to answer, so the run ends here rather than looping
			// on emptiness.
			h.log.Warn("empty assistant turn",
				"thread_id", spec.ThreadID, "turn", turn, "finish_reason", result.finishReason)
			emit(Event{
				Type: EventDone, ThreadID: spec.ThreadID, Turn: turn,
				StopReason: StopEndTurn, FinishReason: result.finishReason,
			})
			return nil
		}

		content, err := AssistantContent(result.text, result.calls)
		if err != nil {
			return fail(turn, CodeInternalError, "The assistant reply could not be encoded.", err)
		}
		if _, err := h.persist(ctx, spec, emit, turn, ports.RoleAssistant, content); err != nil {
			return fail(turn, CodeStorageError, "The assistant reply could not be saved.", err)
		}
		conv = append(conv, ports.ChatMessage{
			Role:      ports.RoleAssistant,
			Content:   result.text,
			ToolCalls: result.calls,
		})

		if len(result.calls) == 0 {
			emit(Event{
				Type: EventDone, ThreadID: spec.ThreadID, Turn: turn,
				StopReason: StopEndTurn, FinishReason: result.finishReason,
			})
			return nil
		}

		for _, call := range result.calls {
			msg, err := h.dispatch(ctx, spec, emit, turn, call)
			if err != nil {
				return fail(turn, CodeStorageError, "A tool result could not be saved.", err)
			}
			conv = append(conv, msg)
			if err := ctx.Err(); err != nil {
				return fail(turn, CodeCanceled, "The run was cancelled.", err)
			}
		}
	}

	// The turn budget ran out with tools still in flight. Their results are
	// persisted, so the thread stays a valid conversation the next run can
	// continue from.
	emit(Event{Type: EventDone, ThreadID: spec.ThreadID, Turn: h.maxTurns, StopReason: StopMaxTurns})
	return nil
}

// turnResult is one assistant response.
type turnResult struct {
	text         string
	calls        []ports.ToolCall
	usage        ports.Usage
	finishReason string
}

// invoke streams one provider call, forwarding deltas as events.
//
// Deltas reach the caller as they arrive but the turn is only persisted once
// it is whole, so a stream that fails leaves the user having watched text that
// is not in the thread. That is the intended trade: a partial assistant
// message on disk would be indistinguishable from a real one on the next run.
//
// Whatever usage the provider reported is metered even when the stream then
// failed — tokens the provider counted are tokens the org spent — which is why
// it is read off the stream rather than off the terminating event a failed
// stream never produces.
func (h *Harness) invoke(
	ctx context.Context,
	spec RunSpec,
	emit func(Event),
	turn int,
	system string,
	conv []ports.ChatMessage,
	reservation ports.LLMReservation,
) (turnResult, error) {
	var result turnResult

	messages := make([]ports.ChatMessage, 0, len(conv)+1)
	if system != "" {
		messages = append(messages, ports.ChatMessage{Role: ports.RoleSystem, Content: system})
	}
	messages = append(messages, conv...)

	stream, err := h.provider.StreamChat(ctx, ports.ChatRequest{
		Model:    h.model,
		Messages: messages,
		Tools:    h.tools.Defs(),
	})
	if err != nil {
		releaseReservation(reservation)
		return result, err
	}
	defer stream.Close()

	var text []byte
	seen := map[string]bool{}
	sawDone := false
	for {
		ev, ok := stream.Next()
		if !ok {
			break
		}
		switch ev.Kind {
		case ports.StreamText:
			text = append(text, ev.Text...)
			emit(Event{Type: EventTextDelta, ThreadID: spec.ThreadID, Turn: turn, Text: ev.Text})
		case ports.StreamToolCall:
			call := ev.ToolCall
			// Every call needs an id of its own: its result answers by id, and
			// a missing or repeated one makes the next request unsendable.
			if call.ID == "" || seen[call.ID] {
				call.ID = h.ids.New()
			}
			seen[call.ID] = true
			result.calls = append(result.calls, call)
		case ports.StreamDone:
			sawDone = true
			result.finishReason = ev.FinishReason
		}
	}
	result.text = string(text)
	result.usage = stream.Usage()

	usageReported := sawDone || result.usage.InputTokens != 0 || result.usage.OutputTokens != 0
	if reporting, ok := stream.(ports.UsageReportingStream); ok {
		usageReported = reporting.UsageReported()
	}
	meterErr := h.meter(ctx, spec, emit, turn, reservation, result.usage, usageReported)
	return result, errors.Join(stream.Err(), meterErr)
}

func (h *Harness) reserveInvocation(ctx context.Context, orgID string) (ports.LLMReservation, error) {
	if ent, ok := h.ent.(ports.ReservingEntitlements); ok {
		return ent.ReserveLLMInvocation(ctx, orgID)
	}
	return nil, h.ent.CheckLLMInvocation(ctx, orgID)
}

func releaseReservation(reservation ports.LLMReservation) {
	if reservation != nil {
		reservation.Release()
	}
}

// meter reports the provider's token count to the entitlements engine. It uses
// a context detached from the run: a cancelled run still spent its tokens, and
// dropping them would let a caller cancel their way out of the bill.
func (h *Harness) meter(
	ctx context.Context,
	spec RunSpec,
	emit func(Event),
	turn int,
	reservation ports.LLMReservation,
	usage ports.Usage,
	usageReported bool,
) error {
	if !usageReported {
		releaseReservation(reservation)
		return nil
	}

	accountingCtx := context.WithoutCancel(ctx)
	var recordErr error
	method := "record"
	if reservation != nil {
		method = "settle"
		recordErr = reservation.Settle(accountingCtx, usage.InputTokens, usage.OutputTokens)
		if recordErr != nil {
			// Settle is single-use, but Release is idempotent and also covers an
			// extension implementation that leaves its hold open on failure.
			reservation.Release()
		}
	} else {
		for attempt := 1; attempt <= usageRecordAttempts; attempt++ {
			recordErr = h.ent.RecordTokenUsage(accountingCtx, spec.OrgID, usage.InputTokens, usage.OutputTokens)
			if recordErr == nil {
				break
			}
		}
	}

	if recordErr != nil {
		trailErr := h.persistPendingUsage(spec, turn, usage, method, recordErr)
		h.log.Error("token usage pending reconciliation",
			"org_id", spec.OrgID, "thread_id", spec.ThreadID, "turn", turn,
			"method", method, "err", recordErr, "trail_err", trailErr)
		if trailErr != nil {
			return errors.Join(recordErr, fmt.Errorf("persist pending token usage: %w", trailErr))
		}
		return nil
	}
	if usage.InputTokens != 0 || usage.OutputTokens != 0 {
		emit(Event{Type: EventUsage, ThreadID: spec.ThreadID, Turn: turn, Usage: &usage})
	}
	return nil
}

type pendingUsageRecord struct {
	ID           string `json:"id"`
	OrgID        string `json:"org_id"`
	ProjectID    string `json:"project_id"`
	ThreadID     string `json:"thread_id"`
	Turn         int    `json:"turn"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	Method       string `json:"method"`
	Error        string `json:"error"`
}

func (h *Harness) persistPendingUsage(spec RunSpec, turn int, usage ports.Usage, method string, recordErr error) error {
	record := pendingUsageRecord{
		ID: h.ids.New(), OrgID: spec.OrgID, ProjectID: spec.ProjectID,
		ThreadID: spec.ThreadID, Turn: turn,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens,
		Method: method, Error: recordErr.Error(),
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	h.usageMu.Lock()
	defer h.usageMu.Unlock()
	dir := layout.OrgProteanDir(h.ctxb.dataDir, spec.OrgID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(dir, pendingUsageFileName), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

// dispatch executes one tool call and persists its result.
//
// A tool that fails, or that the model invented, produces a tool result saying
// so rather than an error: the model gets to see what went wrong and try
// something else.
func (h *Harness) dispatch(ctx context.Context, spec RunSpec, emit func(Event), turn int, call ports.ToolCall) (ports.ChatMessage, error) {
	emit(Event{
		Type:        EventToolCallStarted,
		ThreadID:    spec.ThreadID,
		Turn:        turn,
		ToolCallID:  call.ID,
		ToolName:    call.Name,
		ArgsPreview: preview(string(call.Arguments)),
	})

	result, isError := h.execute(ctx, call)
	if result == "" {
		result = emptyToolResult
	}

	emit(Event{
		Type:          EventToolCallFinished,
		ThreadID:      spec.ThreadID,
		Turn:          turn,
		ToolCallID:    call.ID,
		ToolName:      call.Name,
		ResultPreview: preview(result),
		IsError:       isError,
	})

	content, err := ToolResultContent(call.ID, call.Name, result, isError)
	if err != nil {
		return ports.ChatMessage{}, err
	}
	if _, err := h.persist(ctx, spec, emit, turn, ports.RoleTool, content); err != nil {
		return ports.ChatMessage{}, err
	}
	return ports.ChatMessage{Role: ports.RoleTool, Content: result, ToolCallID: call.ID}, nil
}

func (h *Harness) execute(ctx context.Context, call ports.ToolCall) (string, bool) {
	tool, err := h.tools.Lookup(call.Name)
	if err != nil {
		return ToolErrorPrefix + fmt.Sprintf("there is no tool named %q. Available tools: %v.", call.Name, h.tools.Names()), true
	}
	out, err := tool.Execute(ctx, call.Arguments)
	if err != nil {
		return ToolErrorPrefix + err.Error(), true
	}
	return out, false
}

// persist appends one message and announces it.
func (h *Harness) persist(ctx context.Context, spec RunSpec, emit func(Event), turn int, role string, content json.RawMessage) (ports.Message, error) {
	msg, err := h.threads.AppendMessage(ctx, spec.OrgID, spec.ProjectID, spec.ThreadID, role, content)
	if err != nil {
		return ports.Message{}, err
	}
	emit(Event{
		Type:      EventMessagePersisted,
		ThreadID:  spec.ThreadID,
		Turn:      turn,
		MessageID: msg.ID,
		Role:      role,
	})
	return msg, nil
}

// history reads the thread back as provider messages.
//
// It goes through the content codec rather than a parallel in-memory copy, so
// the run starts from exactly what is on disk, and through RepairToolCalls,
// because a thread whose last run was cut short can hold a tool call the
// provider would refuse to see unanswered. The turns this run produces are
// then appended in memory: they were just written and need no repair.
func (h *Harness) history(ctx context.Context, spec RunSpec) ([]ports.ChatMessage, error) {
	stored, err := h.threads.ListMessages(ctx, spec.OrgID, spec.ProjectID, spec.ThreadID)
	if err != nil {
		return nil, err
	}
	chat, err := MessagesToChat(stored)
	if err != nil {
		return nil, err
	}
	return RepairToolCalls(chat), nil
}

// systemPrompt assembles the instruction context for one invocation. The
// thread is re-read every time because its task list is the model's own
// scratchpad and a tool may have rewritten it during the last turn.
func (h *Harness) systemPrompt(ctx context.Context, spec RunSpec) (string, error) {
	thread, err := h.threads.GetThread(ctx, spec.OrgID, spec.ProjectID, spec.ThreadID)
	if err != nil {
		return "", err
	}
	return h.ctxb.SystemPrompt(ContextInput{
		OrgID:     spec.OrgID,
		ProjectID: spec.ProjectID,
		UserID:    spec.UserID,
		Tasks:     thread.Tasks,
	}), nil
}

func validateSpec(spec RunSpec) error {
	if err := layout.CheckID("org id", spec.OrgID); err != nil {
		return err
	}
	if err := layout.CheckID("project id", spec.ProjectID); err != nil {
		return err
	}
	if err := layout.CheckID("thread id", spec.ThreadID); err != nil {
		return err
	}
	if spec.UserID != "" {
		if err := layout.CheckID("user id", spec.UserID); err != nil {
			return err
		}
	}
	if spec.UserMessage == "" {
		return errors.New("harness: user message is empty")
	}
	return nil
}

// mustText encodes a text-only body. The only failure mode of EncodeContent is
// an empty block list, which a non-empty string cannot produce.
func mustText(text string) json.RawMessage {
	content, err := TextContent(text)
	if err != nil {
		panic(err)
	}
	return content
}

func entitlementCode(err error) string {
	var e *ports.EntitlementError
	if errors.As(err, &e) {
		return e.Code
	}
	return CodeInternalError
}

func entitlementMessage(err error) string {
	var e *ports.EntitlementError
	if errors.As(err, &e) {
		return e.Message
	}
	return "The organization's entitlements could not be checked."
}
