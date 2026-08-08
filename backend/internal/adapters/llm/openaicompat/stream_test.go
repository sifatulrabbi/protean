package openaicompat

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// recordedStream is a captured OpenRouter response: text deltas, one tool call
// whose arguments arrive in four fragments, an OpenRouter keepalive comment,
// a finish_reason chunk with no choices content, and a usage-bearing final
// chunk after it.
const recordedStream = `: OPENROUTER PROCESSING

data: {"id":"gen-1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"Let me "},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"content":"check."},"finish_reason":null}]}

: OPENROUTER PROCESSING

data: {"id":"gen-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_abc","type":"function","function":{"name":"Grep","arguments":""}}]},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"pat"}}]},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"tern\":\"TODO\""}}]},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":null}]}

data: {"id":"gen-1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"gen-1","choices":[],"usage":{"prompt_tokens":1240,"completion_tokens":57,"total_tokens":1297}}

data: [DONE]

`

func collect(t *testing.T, body string) ([]ports.StreamEvent, error) {
	t.Helper()
	s := newStream(io.NopCloser(strings.NewReader(body)))
	defer s.Close()

	var events []ports.StreamEvent
	for {
		ev, ok := s.Next()
		if !ok {
			break
		}
		events = append(events, ev)
	}
	return events, s.Err()
}

func TestStreamRecordedResponse(t *testing.T) {
	events, err := collect(t, recordedStream)
	if err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	want := []ports.StreamEvent{
		{Kind: ports.StreamText, Text: "Let me "},
		{Kind: ports.StreamText, Text: "check."},
		{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{ID: "call_abc", Name: "Grep", Arguments: []byte(`{"pattern":"TODO"}`)}},
		{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 1240, OutputTokens: 57}, FinishReason: "tool_calls"},
	}
	if len(events) != len(want) {
		t.Fatalf("got %d events %v, want %d", len(events), events, len(want))
	}
	for i, w := range want {
		got := events[i]
		if got.Kind != w.Kind || got.Text != w.Text || got.FinishReason != w.FinishReason || got.Usage != w.Usage {
			t.Errorf("event %d = %+v, want %+v", i, got, w)
		}
		if got.ToolCall.ID != w.ToolCall.ID || got.ToolCall.Name != w.ToolCall.Name ||
			string(got.ToolCall.Arguments) != string(w.ToolCall.Arguments) {
			t.Errorf("event %d tool call = %+v, want %+v", i, got.ToolCall, w.ToolCall)
		}
	}
}

func TestStreamParallelToolCalls(t *testing.T) {
	body := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"One","arguments":"{\"x\":1"}},{"index":1,"id":"b","function":{"name":"Two","arguments":"{\"y\":"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"2}"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3}}

data: [DONE]
`
	events, err := collect(t, body)
	if err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if len(events) != 3 {
		t.Fatalf("got %d events, want 3: %+v", len(events), events)
	}
	if got := events[0].ToolCall; got.Name != "One" || string(got.Arguments) != `{"x":1}` {
		t.Errorf("call 0 = %+v", got)
	}
	if got := events[1].ToolCall; got.Name != "Two" || string(got.Arguments) != `{"y":2}` {
		t.Errorf("call 1 = %+v", got)
	}
}

func TestStreamEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    []ports.StreamEvent
		wantErr string
		wantIs  error
	}{
		{
			name:    "finish reason without the requested usage is truncated",
			body:    "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n",
			wantErr: "ended before the model finished",
			wantIs:  ErrTruncatedStream,
		},
		{
			name: "usage only",
			body: "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4}}\n\ndata: [DONE]\n",
			want: []ports.StreamEvent{
				{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 3, OutputTokens: 4}},
			},
		},
		{
			name: "null content is not a delta",
			body: "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\",\"content\":null}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":0,\"completion_tokens\":0}}\n\ndata: [DONE]\n",
			want: []ports.StreamEvent{{Kind: ports.StreamDone}},
		},
		{
			name: "crlf line endings",
			body: "data: {\"choices\":[{\"delta\":{\"content\":\"a\"}}]}\r\n\r\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\r\n\r\ndata: [DONE]\r\n",
			want: []ports.StreamEvent{
				{Kind: ports.StreamText, Text: "a"},
				{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 1, OutputTokens: 1}},
			},
		},
		{
			name:    "unnamed accumulated tool call is malformed",
			body:    "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"x\",\"function\":{\"arguments\":\"{}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n",
			wantErr: "missing function name",
			wantIs:  ErrMalformedToolCall,
		},
		{
			name: "argument-less call becomes an empty object",
			body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"x\",\"function\":{\"name\":\"T\"}}]}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n",
			want: []ports.StreamEvent{
				{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{ID: "x", Name: "T", Arguments: []byte("{}")}},
				{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 1, OutputTokens: 1}},
			},
		},
		{
			// Half a JSON object is half an instruction. Dispatching the tool
			// with what did arrive is how a search becomes a delete.
			name:    "half-received arguments fail the turn",
			body:    "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"x\",\"function\":{\"name\":\"Bash\",\"arguments\":\"{\\\"command\\\":\\\"rm -rf /tm\"}}]}}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n",
			wantErr: "arguments are not valid JSON",
			wantIs:  ErrMalformedToolCall,
		},
		{
			name:    "a body that stops without an ending marker is truncated",
			body:    "data: {\"choices\":[{\"delta\":{\"content\":\"half an ans\"}}]}\n\n",
			wantErr: "ended before the model finished",
			wantIs:  ErrTruncatedStream,
		},
		{
			name: "a fragment carrying only arguments continues the call in flight",
			body: "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"x\",\"function\":{\"name\":\"T\",\"arguments\":\"{\\\"a\\\":\"}}]}}]}\n\ndata: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"function\":{\"arguments\":\"1}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1}}\n\ndata: [DONE]\n",
			want: []ports.StreamEvent{
				{Kind: ports.StreamToolCall, ToolCall: ports.ToolCall{ID: "x", Name: "T", Arguments: []byte(`{"a":1}`)}},
				{Kind: ports.StreamDone, Usage: ports.Usage{InputTokens: 1, OutputTokens: 1}, FinishReason: "tool_calls"},
			},
		},
		{
			name:    "provider error chunk",
			body:    "data: {\"error\":{\"message\":\"rate limited\",\"type\":\"rate_limit\"}}\n\ndata: [DONE]\n",
			wantErr: "rate limited",
		},
		{
			name:    "undecodable chunk",
			body:    "data: {not json}\n",
			wantErr: "decode stream chunk",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			events, err := collect(t, tc.body)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("Err() = %v, want it to contain %q", err, tc.wantErr)
				}
				if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
					t.Fatalf("Err() = %v, want errors.Is(_, %v)", err, tc.wantIs)
				}
				// Deltas already handed over cannot be unsent, but a failed
				// stream must never claim it finished.
				for _, ev := range events {
					if ev.Kind == ports.StreamDone || ev.Kind == ports.StreamToolCall {
						t.Errorf("a failed stream produced a %s event", ev.Kind)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Err() = %v, want nil", err)
			}
			if len(events) != len(tc.want) {
				t.Fatalf("got %d events %+v, want %d", len(events), events, len(tc.want))
			}
			for i, w := range tc.want {
				got := events[i]
				if got.Kind != w.Kind || got.Text != w.Text || got.Usage != w.Usage || got.FinishReason != w.FinishReason {
					t.Errorf("event %d = %+v, want %+v", i, got, w)
				}
				if got.ToolCall.ID != w.ToolCall.ID || got.ToolCall.Name != w.ToolCall.Name ||
					string(got.ToolCall.Arguments) != string(w.ToolCall.Arguments) {
					t.Errorf("event %d tool call = %+v, want %+v", i, got.ToolCall, w.ToolCall)
				}
			}
		})
	}
}

func TestStreamKeepsIndexlessCallsOutOfTheIndexedSlots(t *testing.T) {
	// An indexless fragment that names a function starts a call of its own. It
	// must not occupy a slot a later indexed fragment will ask for, or the two
	// calls merge and both are wrong.
	body := `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"Zero","arguments":"{\"n\":0}"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"id":"loose","function":{"name":"Loose","arguments":"{\"n\":9}"}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":1,"id":"b","function":{"name":"One","arguments":"{\"n\":1}"}}]},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":8,"completion_tokens":3}}

data: [DONE]
`
	events, err := collect(t, body)
	if err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}

	want := []ports.ToolCall{
		{ID: "a", Name: "Zero", Arguments: []byte(`{"n":0}`)},
		{ID: "loose", Name: "Loose", Arguments: []byte(`{"n":9}`)},
		{ID: "b", Name: "One", Arguments: []byte(`{"n":1}`)},
	}
	var got []ports.ToolCall
	for _, ev := range events {
		if ev.Kind == ports.StreamToolCall {
			got = append(got, ev.ToolCall)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("got %d tool calls %+v, want %d", len(got), got, len(want))
	}
	for i, w := range want {
		if got[i].ID != w.ID || got[i].Name != w.Name || string(got[i].Arguments) != string(w.Arguments) {
			t.Errorf("call %d = %+v, want %+v", i, got[i], w)
		}
	}
}

func TestStreamUsageSurvivesAFailure(t *testing.T) {
	// The provider counted the tokens and then the connection died. The org
	// spent them, so the harness has to be able to read them.
	s := newStream(&failingReader{
		head: "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":11,\"completion_tokens\":22}}\n\n",
		err:  errors.New("connection reset"),
	})
	defer s.Close()

	for {
		if _, ok := s.Next(); !ok {
			break
		}
	}
	if s.Err() == nil {
		t.Fatal("Err() = nil, want the read failure")
	}
	if got := s.Usage(); got != (ports.Usage{InputTokens: 11, OutputTokens: 22}) {
		t.Fatalf("Usage() = %+v, want the tokens the provider reported", got)
	}
}

func TestStreamAssemblesMultiLineDataEvent(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\n" +
		"data: \"content\":\"joined\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n" +
		"data: [DONE]\n\n"

	events, err := collect(t, body)
	if err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if len(events) != 2 || events[0].Kind != ports.StreamText || events[0].Text != "joined" ||
		events[1].Kind != ports.StreamDone || events[1].Usage != (ports.Usage{InputTokens: 2, OutputTokens: 1}) ||
		events[1].FinishReason != "stop" {
		t.Fatalf("events = %+v, want joined text and a metered done event", events)
	}
}

func TestStreamHandlesByteAtATimeUTF8(t *testing.T) {
	body := "data: {\"choices\":[{\"delta\":{\"content\":\"সুস্থ\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1}}\n\n" +
		"data: [DONE]\n\n"
	s := newStream(&oneByteReader{data: []byte(body)})
	defer s.Close()

	var text strings.Builder
	for {
		ev, ok := s.Next()
		if !ok {
			break
		}
		if ev.Kind == ports.StreamText {
			text.WriteString(ev.Text)
		}
	}
	if err := s.Err(); err != nil {
		t.Fatalf("Err() = %v, want nil", err)
	}
	if got := text.String(); got != "সুস্থ" {
		t.Fatalf("text = %q, want %q", got, "সুস্থ")
	}
}

type oneByteReader struct {
	data []byte
	pos  int
}

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	p[0] = r.data[r.pos]
	r.pos++
	return 1, nil
}

func (r *oneByteReader) Close() error { return nil }

// failingReader fails partway through, standing in for a connection that died.
type failingReader struct {
	head string
	err  error
	n    int
}

func (r *failingReader) Read(p []byte) (int, error) {
	if r.n < len(r.head) {
		n := copy(p, r.head[r.n:])
		r.n += n
		return n, nil
	}
	return 0, r.err
}

func (r *failingReader) Close() error { return nil }

func TestStreamSurfacesReadFailure(t *testing.T) {
	boom := errors.New("connection reset")
	s := newStream(&failingReader{head: "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n", err: boom})
	defer s.Close()

	ev, ok := s.Next()
	if !ok || ev.Kind != ports.StreamText {
		t.Fatalf("first event = %+v, %v", ev, ok)
	}
	if _, ok := s.Next(); ok {
		t.Fatal("want the stream to stop after the read failure")
	}
	if !errors.Is(s.Err(), boom) {
		t.Fatalf("Err() = %v, want %v", s.Err(), boom)
	}
}

func TestStreamCloseIsIdempotent(t *testing.T) {
	s := newStream(io.NopCloser(strings.NewReader("data: [DONE]\n")))
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}
