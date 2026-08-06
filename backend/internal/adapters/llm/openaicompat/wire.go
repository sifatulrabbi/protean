package openaicompat

import (
	"encoding/json"
	"fmt"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// The OpenAI /chat/completions wire shapes, kept private to this package.
// Only the fields Protean sends or reads are modelled; everything else in a
// response is ignored on purpose, so a provider adding fields cannot break the
// adapter.

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []wireMessage  `json:"messages"`
	Tools         []wireTool     `json:"tools,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions *streamOptions `json:"stream_options,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type wireMessage struct {
	Role string `json:"role"`

	// Content is a pointer so an assistant message that only calls tools can
	// send JSON null, which is the shape the API itself returns for it.
	Content    *string        `json:"content"`
	ToolCalls  []wireToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type wireToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function wireToolCallFunc `json:"function"`
}

type wireToolCallFunc struct {
	Name string `json:"name"`
	// Arguments is a JSON *string* on the wire, not an object.
	Arguments string `json:"arguments"`
}

type wireTool struct {
	Type     string       `json:"type"`
	Function wireToolSpec `json:"function"`
}

type wireToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// chunk is one `data:` payload of a streaming response.
type chunk struct {
	Choices []chunkChoice `json:"choices"`
	Usage   *wireUsage    `json:"usage"`
	Error   *wireError    `json:"error"`
}

type chunkChoice struct {
	Index        int        `json:"index"`
	Delta        chunkDelta `json:"delta"`
	FinishReason *string    `json:"finish_reason"`
	Error        *wireError `json:"error"`
}

type chunkDelta struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls []toolCallDelta `json:"tool_calls"`
}

// toolCallDelta is a fragment of a tool call. Only the first fragment carries
// the id and the name; the rest carry more argument text.
type toolCallDelta struct {
	Index    *int                 `json:"index"`
	ID       string               `json:"id"`
	Type     string               `json:"type"`
	Function *toolCallDeltaFuncts `json:"function"`
}

type toolCallDeltaFuncts struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type wireUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
}

type wireError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

func (e *wireError) Error() string {
	if e.Type != "" {
		return fmt.Sprintf("%s: %s", e.Type, e.Message)
	}
	return e.Message
}

// emptyArguments is what a tool call carries when the model produced no
// arguments at all: an empty object, never an empty string, because the tool
// on the other side unmarshals it.
const emptyArguments = "{}"

// encodeMessages converts port messages to the wire shape.
func encodeMessages(msgs []ports.ChatMessage) []wireMessage {
	out := make([]wireMessage, 0, len(msgs))
	for _, m := range msgs {
		wm := wireMessage{Role: m.Role, ToolCallID: m.ToolCallID}
		if len(m.ToolCalls) > 0 {
			wm.ToolCalls = make([]wireToolCall, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				args := string(tc.Arguments)
				if args == "" {
					args = emptyArguments
				}
				wm.ToolCalls = append(wm.ToolCalls, wireToolCall{
					ID:       tc.ID,
					Type:     "function",
					Function: wireToolCallFunc{Name: tc.Name, Arguments: args},
				})
			}
		}
		// An assistant turn that only calls tools has no content; anything else
		// sends its text, even when empty.
		if m.Content != "" || len(wm.ToolCalls) == 0 {
			content := m.Content
			wm.Content = &content
		}
		out = append(out, wm)
	}
	return out
}

// encodeTools converts port tool definitions to the wire shape.
func encodeTools(defs []ports.ToolDef) []wireTool {
	if len(defs) == 0 {
		return nil
	}
	out := make([]wireTool, 0, len(defs))
	for _, d := range defs {
		out = append(out, wireTool{
			Type: "function",
			Function: wireToolSpec{
				Name:        d.Name,
				Description: d.Description,
				Parameters:  d.Parameters,
			},
		})
	}
	return out
}
