package openaicompat

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// doneMarker ends an OpenAI-compatible SSE stream.
const doneMarker = "[DONE]"

// readerBufferSize is the initial line buffer. bufio.Reader grows as needed,
// so a single huge argument fragment cannot overflow it — unlike a Scanner,
// which is why this is a Reader.
const readerBufferSize = 64 << 10

// drainLimit is how much of an abandoned body is read before Close, so the
// keep-alive connection goes back to the pool instead of being torn down.
const drainLimit = 64 << 10

// Stream rejections. Match with errors.Is.
var (
	// ErrTruncatedStream is a response that ended without the provider ever
	// saying it had finished — no [DONE] marker and no finish reason. What
	// arrived is an unknown fraction of the answer, so it is not the answer.
	ErrTruncatedStream = errors.New("openaicompat: stream ended before the model finished")

	// ErrMalformedToolCall is a tool call whose accumulated arguments are not
	// JSON. It fails the turn instead of dispatching the tool: the arguments
	// that did arrive are a prefix of an unknown whole, and guessing at what
	// the model meant is how a search becomes a delete.
	ErrMalformedToolCall = errors.New("openaicompat: tool call arguments are not valid JSON")
)

// stream is the pull iterator over one SSE response body.
//
// Tool calls arrive as fragments — the first carries the id and the function
// name, later ones carry more argument text — so they are accumulated and
// emitted, complete, once the stream finishes. Everything the parser has
// produced but the caller has not taken yet waits in pending.
type stream struct {
	body io.ReadCloser
	br   *bufio.Reader

	pending []ports.StreamEvent
	acc     *toolCallAccumulator

	usage        ports.Usage
	finishReason string
	sawDone      bool

	finished bool
	closed   bool
	err      error
}

var _ ports.ChatStream = (*stream)(nil)

func newStream(body io.ReadCloser) *stream {
	return &stream{
		body: body,
		br:   bufio.NewReaderSize(body, readerBufferSize),
		acc:  newToolCallAccumulator(),
	}
}

// Next returns the next event, or false at the end of the stream and on
// failure.
func (s *stream) Next() (ports.StreamEvent, bool) {
	for {
		if len(s.pending) > 0 {
			ev := s.pending[0]
			s.pending = s.pending[1:]
			return ev, true
		}
		if s.finished || s.err != nil {
			return ports.StreamEvent{}, false
		}
		s.readMore()
	}
}

// Usage is what the provider reported, whether or not the stream then failed.
func (s *stream) Usage() ports.Usage { return s.usage }

// Err reports why the stream stopped early, or nil when it ended cleanly.
func (s *stream) Err() error { return s.err }

// Close releases the response body. It is idempotent.
func (s *stream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	// Draining what is left lets the transport reuse the connection. The cap
	// stops an abandoned long answer from being read out in full first.
	_, _ = io.CopyN(io.Discard, s.br, drainLimit)
	return s.body.Close()
}

// readMore consumes SSE lines until it has produced at least one event or the
// stream ends.
func (s *stream) readMore() {
	for {
		line, err := s.br.ReadString('\n')
		if len(line) > 0 {
			if done := s.handleLine(line); done || len(s.pending) > 0 || s.err != nil {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.finish()
				return
			}
			s.fail(err)
			return
		}
	}
}

// handleLine parses one SSE line and reports whether the stream ended.
func (s *stream) handleLine(line string) bool {
	line = strings.TrimRight(line, "\r\n")
	switch {
	case line == "":
		return false
	case strings.HasPrefix(line, ":"):
		// A comment. OpenRouter sends these as keepalives while it queues.
		return false
	}

	field, value, ok := strings.Cut(line, ":")
	if !ok || field != "data" {
		// event:, id:, retry: and anything unknown carry nothing Protean needs.
		// Several data: lines forming one event are not joined either: no
		// provider Protean talks to splits a JSON chunk that way.
		return false
	}
	payload := strings.TrimSpace(value)
	if payload == doneMarker {
		s.sawDone = true
		s.finish()
		return true
	}
	if payload == "" {
		return false
	}

	var c chunk
	if err := json.Unmarshal([]byte(payload), &c); err != nil {
		s.fail(fmt.Errorf("openaicompat: decode stream chunk: %w", err))
		return true
	}
	s.handleChunk(c)
	return s.err != nil
}

func (s *stream) handleChunk(c chunk) {
	if c.Error != nil {
		s.fail(fmt.Errorf("openaicompat: provider error: %s", c.Error.Error()))
		return
	}
	if c.Usage != nil {
		s.usage = ports.Usage{
			InputTokens:  c.Usage.PromptTokens,
			OutputTokens: c.Usage.CompletionTokens,
		}
	}
	for _, choice := range c.Choices {
		if choice.Error != nil {
			s.fail(fmt.Errorf("openaicompat: provider error: %s", choice.Error.Error()))
			return
		}
		delta := choice.Delta
		if delta.Content != "" {
			s.pending = append(s.pending, ports.StreamEvent{Kind: ports.StreamText, Text: delta.Content})
		}
		for _, tc := range delta.ToolCalls {
			s.acc.add(tc)
		}
		if choice.FinishReason != nil && *choice.FinishReason != "" {
			s.finishReason = *choice.FinishReason
		}
	}
}

// finish queues the accumulated tool calls and the terminating done event.
func (s *stream) finish() {
	if s.finished {
		return
	}
	// A body that stopped without either ending marker was cut off. Reporting
	// that as a complete answer is the one failure the caller cannot detect
	// for itself.
	if !s.sawDone && s.finishReason == "" {
		s.fail(ErrTruncatedStream)
		return
	}

	calls, err := s.acc.calls()
	if err != nil {
		s.fail(err)
		return
	}

	s.finished = true
	for _, call := range calls {
		s.pending = append(s.pending, ports.StreamEvent{Kind: ports.StreamToolCall, ToolCall: call})
	}
	s.pending = append(s.pending, ports.StreamEvent{
		Kind:         ports.StreamDone,
		Usage:        s.usage,
		FinishReason: s.finishReason,
	})
}

// fail records the first failure and drops anything still queued: a caller
// must not act on half a turn. The usage counter survives, because tokens the
// provider counted are tokens the org spent.
func (s *stream) fail(err error) {
	if s.err == nil {
		s.err = err
	}
	s.pending = nil
	s.finished = true
}

// toolCallAccumulator joins tool-call fragments back into whole calls.
//
// Fragments are keyed by their index. A provider that omits the index is
// matched on the id when it repeats one, and a fragment carrying nothing but
// argument text continues the call already in flight — which is the only
// reading of it that does not throw the text away.
type toolCallAccumulator struct {
	byIndex map[int]*toolCallBuilder
	order   []*toolCallBuilder
}

type toolCallBuilder struct {
	id   string
	name string
	args strings.Builder
}

func newToolCallAccumulator() *toolCallAccumulator {
	return &toolCallAccumulator{byIndex: map[int]*toolCallBuilder{}}
}

func (a *toolCallAccumulator) add(d toolCallDelta) {
	b := a.builderFor(d)
	if b == nil {
		return
	}
	if d.ID != "" {
		b.id = d.ID
	}
	if d.Function != nil {
		if d.Function.Name != "" {
			b.name = d.Function.Name
		}
		b.args.WriteString(d.Function.Arguments)
	}
}

func (a *toolCallAccumulator) builderFor(d toolCallDelta) *toolCallBuilder {
	if d.Index != nil {
		b, ok := a.byIndex[*d.Index]
		if !ok {
			b = &toolCallBuilder{}
			a.byIndex[*d.Index] = b
			a.order = append(a.order, b)
		}
		return b
	}
	if d.ID != "" {
		for _, b := range a.order {
			if b.id == d.ID {
				return b
			}
		}
	}
	// Neither an index nor a known id. A fragment that names a function starts
	// a call; one carrying only arguments continues the last one. Builders made
	// here stay out of byIndex, so an indexed fragment can never be handed one
	// of them by accident.
	if d.ID == "" && (d.Function == nil || d.Function.Name == "") {
		if len(a.order) == 0 {
			return nil
		}
		return a.order[len(a.order)-1]
	}
	b := &toolCallBuilder{}
	a.order = append(a.order, b)
	return b
}

// calls returns the accumulated calls in the order they first appeared.
// Fragments that never produced a name are dropped: they cannot be dispatched.
func (a *toolCallAccumulator) calls() ([]ports.ToolCall, error) {
	out := make([]ports.ToolCall, 0, len(a.order))
	for _, b := range a.order {
		if b.name == "" {
			continue
		}
		args := strings.TrimSpace(b.args.String())
		switch {
		case args == "":
			// No arguments at all is what a parameterless tool looks like.
			args = emptyArguments
		case !json.Valid([]byte(args)):
			return nil, fmt.Errorf("%w: %s", ErrMalformedToolCall, b.name)
		}
		out = append(out, ports.ToolCall{
			ID:        b.id,
			Name:      b.name,
			Arguments: json.RawMessage(args),
		})
	}
	return out, nil
}
