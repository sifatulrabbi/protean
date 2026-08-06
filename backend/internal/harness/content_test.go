package harness

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

func TestContentRoundTrip(t *testing.T) {
	calls := []ports.ToolCall{
		{ID: "c1", Name: "Grep", Arguments: json.RawMessage(`{"pattern":"x"}`)},
		{ID: "c2", Name: "ListDir", Arguments: json.RawMessage(`{"path":"."}`)},
	}

	tests := []struct {
		name   string
		encode func() (json.RawMessage, error)
		want   []Block
	}{
		{
			name:   "text",
			encode: func() (json.RawMessage, error) { return TextContent("hello") },
			want:   []Block{{Type: BlockText, Text: "hello"}},
		},
		{
			name:   "assistant with text and calls",
			encode: func() (json.RawMessage, error) { return AssistantContent("on it", calls) },
			want: []Block{
				{Type: BlockText, Text: "on it"},
				{Type: BlockToolCall, ToolCallID: "c1", ToolName: "Grep", Arguments: json.RawMessage(`{"pattern":"x"}`)},
				{Type: BlockToolCall, ToolCallID: "c2", ToolName: "ListDir", Arguments: json.RawMessage(`{"path":"."}`)},
			},
		},
		{
			name:   "assistant with calls only",
			encode: func() (json.RawMessage, error) { return AssistantContent("", calls[:1]) },
			want: []Block{
				{Type: BlockToolCall, ToolCallID: "c1", ToolName: "Grep", Arguments: json.RawMessage(`{"pattern":"x"}`)},
			},
		},
		{
			name:   "tool result",
			encode: func() (json.RawMessage, error) { return ToolResultContent("c1", "Grep", "no matches", false) },
			want:   []Block{{Type: BlockToolResult, ToolCallID: "c1", ToolName: "Grep", Result: "no matches"}},
		},
		{
			name:   "failed tool result",
			encode: func() (json.RawMessage, error) { return ToolResultContent("c1", "Grep", "boom", true) },
			want:   []Block{{Type: BlockToolResult, ToolCallID: "c1", ToolName: "Grep", Result: "boom", IsError: true}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.encode()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !json.Valid(raw) {
				t.Fatalf("encoded content is not valid JSON: %s", raw)
			}
			blocks, err := DecodeContent(raw)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if len(blocks) != len(tc.want) {
				t.Fatalf("got %d blocks %+v, want %d", len(blocks), blocks, len(tc.want))
			}
			for i, w := range tc.want {
				got := blocks[i]
				if got.Type != w.Type || got.Text != w.Text || got.ToolCallID != w.ToolCallID ||
					got.ToolName != w.ToolName || got.Result != w.Result || got.IsError != w.IsError ||
					string(got.Arguments) != string(w.Arguments) {
					t.Errorf("block %d = %+v, want %+v", i, got, w)
				}
			}
		})
	}
}

func TestEncodeContentRejectsEmptyBlocks(t *testing.T) {
	if _, err := EncodeContent(nil); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("EncodeContent(nil) = %v, want ErrInvalidContent", err)
	}
	if _, err := AssistantContent("", nil); !errors.Is(err, ErrInvalidContent) {
		t.Fatalf("AssistantContent with nothing = %v, want ErrInvalidContent", err)
	}
}

func TestDecodeContentRejections(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want error
	}{
		{"not json", `{`, ErrInvalidContent},
		{"wrong shape", `{"v":"one"}`, ErrInvalidContent},
		{"future schema", `{"v":99,"blocks":[]}`, ErrUnsupportedSchema},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := DecodeContent(json.RawMessage(tc.raw)); !errors.Is(err, tc.want) {
				t.Fatalf("DecodeContent = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestDecodeContentAcceptsAVersionlessEnvelope(t *testing.T) {
	blocks, err := DecodeContent(json.RawMessage(`{"blocks":[{"type":"text","text":"hi"}]}`))
	if err != nil {
		t.Fatalf("DecodeContent: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Text != "hi" {
		t.Fatalf("blocks = %+v", blocks)
	}
}

func TestMessagesToChat(t *testing.T) {
	assistant, err := AssistantContent("looking", []ports.ToolCall{
		{ID: "c1", Name: "Grep", Arguments: json.RawMessage(`{"pattern":"x"}`)},
	})
	if err != nil {
		t.Fatalf("AssistantContent: %v", err)
	}
	user, _ := TextContent("find x")
	toolResult, _ := ToolResultContent("c1", "Grep", "a.go:1", false)

	chat, err := MessagesToChat([]ports.Message{
		{ID: "m1", Role: ports.RoleUser, Content: user},
		{ID: "m2", Role: ports.RoleAssistant, Content: assistant},
		{ID: "m3", Role: ports.RoleTool, Content: toolResult},
	})
	if err != nil {
		t.Fatalf("MessagesToChat: %v", err)
	}
	if len(chat) != 3 {
		t.Fatalf("got %d chat messages, want 3: %+v", len(chat), chat)
	}
	if chat[0].Role != ports.RoleUser || chat[0].Content != "find x" {
		t.Errorf("user = %+v", chat[0])
	}
	if chat[1].Role != ports.RoleAssistant || chat[1].Content != "looking" ||
		len(chat[1].ToolCalls) != 1 || chat[1].ToolCalls[0].Name != "Grep" {
		t.Errorf("assistant = %+v", chat[1])
	}
	if chat[2].Role != ports.RoleTool || chat[2].ToolCallID != "c1" || chat[2].Content != "a.go:1" {
		t.Errorf("tool = %+v", chat[2])
	}
}

func TestMessagesToChatSkipsUnknownBlocks(t *testing.T) {
	raw := json.RawMessage(`{"v":1,"blocks":[{"type":"thinking","text":"hmm"},{"type":"text","text":"answer"}]}`)
	chat, err := MessagesToChat([]ports.Message{{ID: "m1", Role: ports.RoleAssistant, Content: raw}})
	if err != nil {
		t.Fatalf("MessagesToChat: %v", err)
	}
	if len(chat) != 1 || chat[0].Content != "answer" {
		t.Fatalf("chat = %+v, want only the text block", chat)
	}
}

func TestMessagesToChatDropsEmptyMessages(t *testing.T) {
	raw := json.RawMessage(`{"v":1,"blocks":[{"type":"thinking","text":"hmm"}]}`)
	chat, err := MessagesToChat([]ports.Message{{ID: "m1", Role: ports.RoleAssistant, Content: raw}})
	if err != nil {
		t.Fatalf("MessagesToChat: %v", err)
	}
	if len(chat) != 0 {
		t.Fatalf("chat = %+v, want nothing", chat)
	}
}

func TestMessagesToChatSplitsSeveralToolResults(t *testing.T) {
	raw, err := EncodeContent([]Block{
		{Type: BlockToolResult, ToolCallID: "c1", ToolName: "A", Result: "one"},
		{Type: BlockToolResult, ToolCallID: "c2", ToolName: "B", Result: "two"},
	})
	if err != nil {
		t.Fatalf("EncodeContent: %v", err)
	}
	chat, err := MessagesToChat([]ports.Message{{ID: "m1", Role: ports.RoleTool, Content: raw}})
	if err != nil {
		t.Fatalf("MessagesToChat: %v", err)
	}
	if len(chat) != 2 || chat[0].ToolCallID != "c1" || chat[1].ToolCallID != "c2" {
		t.Fatalf("chat = %+v", chat)
	}
}

func TestRepairToolCalls(t *testing.T) {
	assistant := func(calls ...string) ports.ChatMessage {
		m := ports.ChatMessage{Role: ports.RoleAssistant, Content: "working"}
		for _, id := range calls {
			m.ToolCalls = append(m.ToolCalls, ports.ToolCall{ID: id, Name: "T", Arguments: json.RawMessage(`{}`)})
		}
		return m
	}
	toolMsg := func(id string) ports.ChatMessage {
		return ports.ChatMessage{Role: ports.RoleTool, ToolCallID: id, Content: "result " + id}
	}
	user := ports.ChatMessage{Role: ports.RoleUser, Content: "go"}

	// shape renders a conversation as "role/id" pairs.
	shape := func(msgs []ports.ChatMessage) []string {
		out := make([]string, 0, len(msgs))
		for _, m := range msgs {
			label := m.Role
			for _, c := range m.ToolCalls {
				label += "+" + c.ID
			}
			if m.ToolCallID != "" {
				label += "=" + m.ToolCallID
			}
			out = append(out, label)
		}
		return out
	}

	tests := []struct {
		name string
		in   []ports.ChatMessage
		want []string
	}{
		{
			name: "a complete conversation is untouched",
			in:   []ports.ChatMessage{user, assistant("c1"), toolMsg("c1")},
			want: []string{"user", "assistant+c1", "tool=c1"},
		},
		{
			name: "an unanswered call gets a synthetic result",
			in:   []ports.ChatMessage{user, assistant("c1")},
			want: []string{"user", "assistant+c1", "tool=c1"},
		},
		{
			name: "only the missing one of several is filled in",
			in:   []ports.ChatMessage{assistant("c1", "c2"), toolMsg("c1")},
			want: []string{"assistant+c1+c2", "tool=c1", "tool=c2"},
		},
		{
			name: "the repair sits before the next user turn",
			in:   []ports.ChatMessage{assistant("c1"), user},
			want: []string{"assistant+c1", "tool=c1", "user"},
		},
		{
			name: "a tool message answering nothing is dropped",
			in:   []ports.ChatMessage{user, toolMsg("ghost")},
			want: []string{"user"},
		},
		{
			name: "a tool message for another call is dropped",
			in:   []ports.ChatMessage{assistant("c1"), toolMsg("c1"), toolMsg("stale")},
			want: []string{"assistant+c1", "tool=c1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shape(RepairToolCalls(tc.in))
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}

	// The synthetic result says what happened, so the model can react to it.
	repaired := RepairToolCalls([]ports.ChatMessage{assistant("c1")})
	if repaired[1].Content != UnansweredToolCallResult {
		t.Errorf("synthetic result = %q", repaired[1].Content)
	}
}

func TestMessagesToChatNamesTheBadMessage(t *testing.T) {
	_, err := MessagesToChat([]ports.Message{{ID: "01JBROKEN", Role: ports.RoleUser, Content: json.RawMessage(`{`)}})
	if err == nil || !strings.Contains(err.Error(), "01JBROKEN") {
		t.Fatalf("err = %v, want it to name the message", err)
	}
}
