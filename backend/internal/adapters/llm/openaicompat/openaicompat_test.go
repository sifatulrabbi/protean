package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

const testKey = "sk-secret-do-not-log"

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestClient(t *testing.T, srv *httptest.Server, mutate func(*Options)) *Client {
	t.Helper()
	opts := Options{
		BaseURL:    srv.URL,
		APIKey:     testKey,
		Model:      "test/model",
		HTTPClient: srv.Client(),
		Logger:     discardLogger(),
	}
	if mutate != nil {
		mutate(&opts)
	}
	client, err := New(opts)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return client
}

func TestNewRejectsIncompleteOptions(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want error
	}{
		{"no key", Options{BaseURL: "https://x", Model: "m"}, ErrNoAPIKey},
		{"no base url", Options{APIKey: "k", Model: "m"}, ErrNoBaseURL},
		{"no model", Options{APIKey: "k", BaseURL: "https://x"}, ErrNoModel},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := New(tc.opts); !errors.Is(err, tc.want) {
				t.Fatalf("New = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestStreamChatRequestShape(t *testing.T) {
	var (
		gotPath   string
		gotHeader http.Header
		gotBody   chatRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotHeader = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	client := newTestClient(t, srv, func(o *Options) {
		o.SiteURL = "https://protean.test"
		o.SiteName = "Protean"
	})

	params := json.RawMessage(`{"type":"object","properties":{"q":{"type":"string"}}}`)
	stream, err := client.StreamChat(context.Background(), ports.ChatRequest{
		Messages: []ports.ChatMessage{
			{Role: ports.RoleSystem, Content: "be useful"},
			{Role: ports.RoleUser, Content: "search"},
			{Role: ports.RoleAssistant, ToolCalls: []ports.ToolCall{{ID: "c1", Name: "Search", Arguments: json.RawMessage(`{"q":"go"}`)}}},
			{Role: ports.RoleTool, ToolCallID: "c1", Content: "found"},
		},
		Tools: []ports.ToolDef{{Name: "Search", Description: "search the web", Parameters: params}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	stream.Close()

	if gotPath != "/chat/completions" {
		t.Errorf("path = %q, want /chat/completions", gotPath)
	}
	if got := gotHeader.Get("Authorization"); got != "Bearer "+testKey {
		t.Errorf("Authorization = %q", got)
	}
	if got := gotHeader.Get("HTTP-Referer"); got != "https://protean.test" {
		t.Errorf("HTTP-Referer = %q", got)
	}
	if got := gotHeader.Get("X-Title"); got != "Protean" {
		t.Errorf("X-Title = %q", got)
	}
	if !gotBody.Stream {
		t.Error("stream = false, want true")
	}
	if gotBody.StreamOptions == nil || !gotBody.StreamOptions.IncludeUsage {
		t.Errorf("stream_options = %+v, want include_usage", gotBody.StreamOptions)
	}
	if gotBody.Model != "test/model" {
		t.Errorf("model = %q, want the configured default", gotBody.Model)
	}
	if len(gotBody.Tools) != 1 || gotBody.Tools[0].Type != "function" ||
		gotBody.Tools[0].Function.Name != "Search" ||
		string(gotBody.Tools[0].Function.Parameters) != string(params) {
		t.Errorf("tools = %+v", gotBody.Tools)
	}

	if len(gotBody.Messages) != 4 {
		t.Fatalf("got %d messages, want 4", len(gotBody.Messages))
	}
	assistant := gotBody.Messages[2]
	if assistant.Content != nil {
		t.Errorf("tool-only assistant content = %q, want null", *assistant.Content)
	}
	if len(assistant.ToolCalls) != 1 || assistant.ToolCalls[0].Function.Arguments != `{"q":"go"}` {
		t.Errorf("assistant tool calls = %+v", assistant.ToolCalls)
	}
	tool := gotBody.Messages[3]
	if tool.ToolCallID != "c1" || tool.Content == nil || *tool.Content != "found" {
		t.Errorf("tool message = %+v", tool)
	}
}

func TestStreamChatOmitsUnsetAttributionHeaders(t *testing.T) {
	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	stream, err := newTestClient(t, srv, nil).StreamChat(context.Background(), ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: ports.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	stream.Close()

	if _, ok := gotHeader["Http-Referer"]; ok {
		t.Error("HTTP-Referer sent although no site url is configured")
	}
	if _, ok := gotHeader["X-Title"]; ok {
		t.Error("X-Title sent although no site name is configured")
	}
}

func TestStreamChatSurfacesHTTPError(t *testing.T) {
	body := strings.Repeat("x", errorBodyLimit+200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		io.WriteString(w, body)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv, nil).StreamChat(context.Background(), ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: ports.RoleUser, Content: "hi"}},
	})
	if err == nil {
		t.Fatal("want an error for a 429 response")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T (%v), want *APIError", err, err)
	}
	if apiErr.StatusCode != http.StatusTooManyRequests {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
	if !strings.HasSuffix(apiErr.Body, "... (truncated)") {
		t.Error("want an oversized body to be truncated")
	}
	if len(apiErr.Body) > errorBodyLimit+len("... (truncated)") {
		t.Errorf("body length = %d, want it capped", len(apiErr.Body))
	}
	// The key authenticates the process; it must never reach a log line.
	if strings.Contains(err.Error(), testKey) {
		t.Error("the api key leaked into the error")
	}
}

func TestStreamChatCancellation(t *testing.T) {
	released := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, _ := w.(http.Flusher)
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		flusher.Flush()
		<-released
	}))
	defer srv.Close()
	defer close(released)

	ctx, cancel := context.WithCancel(context.Background())
	stream, err := newTestClient(t, srv, nil).StreamChat(ctx, ports.ChatRequest{
		Messages: []ports.ChatMessage{{Role: ports.RoleUser, Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}
	defer stream.Close()

	ev, ok := stream.Next()
	if !ok || ev.Text != "partial" {
		t.Fatalf("first event = %+v, %v", ev, ok)
	}

	cancel()
	if _, ok := stream.Next(); ok {
		t.Fatal("want the stream to stop once the context is cancelled")
	}
	if !errors.Is(stream.Err(), context.Canceled) {
		t.Fatalf("Err() = %v, want context.Canceled", stream.Err())
	}
}

func TestStreamChatRejectsEmptyConversation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("the provider must not be called for an empty conversation")
	}))
	defer srv.Close()

	if _, err := newTestClient(t, srv, nil).StreamChat(context.Background(), ports.ChatRequest{}); err == nil {
		t.Fatal("want an error for a request with no messages")
	}
}
