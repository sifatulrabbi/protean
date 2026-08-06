package harness

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sifatulrabbi/protean/backend/internal/ports"
)

// The stored message content schema.
//
// ports.Message.Content is deliberately opaque to the storage port: the
// harness owns what a message is made of. One message holds an ordered list of
// blocks, wrapped in an envelope carrying a schema version so a later slice can
// add block kinds without guessing what an old file meant.
//
//	{"v":1,"blocks":[{"type":"text","text":"hi"}]}
//
// Three block kinds exist today, one per role Protean persists:
//
//   - text        — assistant or user prose.
//   - tool_call   — a call the assistant asked for: id, name, raw arguments.
//     It always sits in the same message as the assistant text
//     that preceded it, mirroring one provider turn.
//   - tool_result — the outcome of one call, in its own message with role
//     "tool": the call id, the tool name, the string the tool
//     returned, and whether that string is a failure report.
//
// Unknown block kinds are preserved by DecodeContent and skipped when a
// message is converted back into provider messages, so a newer writer cannot
// break an older reader.
//
// The API slice (S11) renders threads from these same helpers; it must not
// parse the raw JSON itself.
const ContentVersion = 1

// BlockType discriminates a Block.
type BlockType string

const (
	BlockText       BlockType = "text"
	BlockToolCall   BlockType = "tool_call"
	BlockToolResult BlockType = "tool_result"
)

// Block is one piece of a message. Only the fields belonging to Type are set.
type Block struct {
	Type BlockType `json:"type"`

	// Text is set on text blocks.
	Text string `json:"text,omitempty"`

	// ToolCallID and ToolName are set on tool_call and tool_result blocks.
	ToolCallID string `json:"tool_call_id,omitempty"`
	ToolName   string `json:"tool_name,omitempty"`

	// Arguments is the raw JSON the model produced, set on tool_call blocks.
	Arguments json.RawMessage `json:"arguments,omitempty"`

	// Result is what the tool returned, set on tool_result blocks. IsError
	// marks it as a failure report rather than an answer.
	Result  string `json:"result,omitempty"`
	IsError bool   `json:"is_error,omitempty"`
}

// Content is the envelope written to disk.
type Content struct {
	Version int     `json:"v"`
	Blocks  []Block `json:"blocks"`
}

// Content schema rejections. Match with errors.Is.
var (
	ErrInvalidContent    = errors.New("harness: invalid message content")
	ErrUnsupportedSchema = errors.New("harness: unsupported message content version")
)

// EncodeContent wraps blocks in the current envelope.
func EncodeContent(blocks []Block) (json.RawMessage, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("%w: no blocks", ErrInvalidContent)
	}
	data, err := json.Marshal(Content{Version: ContentVersion, Blocks: blocks})
	if err != nil {
		return nil, fmt.Errorf("harness: encode content: %w", err)
	}
	return data, nil
}

// DecodeContent unwraps a stored message body. A version newer than this build
// understands is an error rather than a guess.
func DecodeContent(raw json.RawMessage) ([]Block, error) {
	var c Content
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidContent, err)
	}
	// Version 0 is a body written before the envelope carried one; it has the
	// same shape as version 1.
	if c.Version > ContentVersion {
		return nil, fmt.Errorf("%w: %d", ErrUnsupportedSchema, c.Version)
	}
	return c.Blocks, nil
}

// TextContent is the body of a plain user or assistant message.
func TextContent(text string) (json.RawMessage, error) {
	return EncodeContent([]Block{{Type: BlockText, Text: text}})
}

// AssistantContent is the body of one assistant turn: its text, then the tool
// calls it asked for. At least one of the two must be present.
func AssistantContent(text string, calls []ports.ToolCall) (json.RawMessage, error) {
	blocks := make([]Block, 0, len(calls)+1)
	if text != "" {
		blocks = append(blocks, Block{Type: BlockText, Text: text})
	}
	for _, c := range calls {
		blocks = append(blocks, Block{
			Type:       BlockToolCall,
			ToolCallID: c.ID,
			ToolName:   c.Name,
			Arguments:  c.Arguments,
		})
	}
	return EncodeContent(blocks)
}

// ToolResultContent is the body of one tool message.
func ToolResultContent(callID, toolName, result string, isError bool) (json.RawMessage, error) {
	return EncodeContent([]Block{{
		Type:       BlockToolResult,
		ToolCallID: callID,
		ToolName:   toolName,
		Result:     result,
		IsError:    isError,
	}})
}

// MessagesToChat converts stored messages into provider messages, in order.
//
// One stored message usually becomes one provider message; a message carrying
// several tool results becomes one provider message each, because that is how
// the wire represents them. Unknown block kinds are skipped, and a message
// that yields nothing at all is dropped rather than sent empty.
func MessagesToChat(msgs []ports.Message) ([]ports.ChatMessage, error) {
	out := make([]ports.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		blocks, err := DecodeContent(m.Content)
		if err != nil {
			return nil, fmt.Errorf("message %s: %w", m.ID, err)
		}
		out = append(out, blocksToChat(m.Role, blocks)...)
	}
	return out, nil
}

func blocksToChat(role string, blocks []Block) []ports.ChatMessage {
	var (
		text    strings.Builder
		calls   []ports.ToolCall
		results []ports.ChatMessage
	)
	for _, b := range blocks {
		switch b.Type {
		case BlockText:
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(b.Text)
		case BlockToolCall:
			calls = append(calls, ports.ToolCall{
				ID:        b.ToolCallID,
				Name:      b.ToolName,
				Arguments: b.Arguments,
			})
		case BlockToolResult:
			results = append(results, ports.ChatMessage{
				Role:       ports.RoleTool,
				Content:    b.Result,
				ToolCallID: b.ToolCallID,
			})
		}
	}

	var out []ports.ChatMessage
	if text.Len() > 0 || len(calls) > 0 {
		out = append(out, ports.ChatMessage{
			Role:      role,
			Content:   text.String(),
			ToolCalls: calls,
		})
	}
	return append(out, results...)
}

// UnansweredToolCallResult is what RepairToolCalls puts in place of a tool
// result that was never written.
const UnansweredToolCallResult = "Tool execution did not complete: the previous run ended before this call returned."

// RepairToolCalls makes a conversation acceptable to the provider.
//
// Every tool call must be answered by a tool message, and every tool message
// must answer a call — the API rejects a conversation where either does not
// hold. A thread can end up violating that without anybody doing anything
// wrong: a run cancelled between the assistant message and the tool result, or
// a disk quota that rejected the result, leaves the call dangling for good,
// because messages are append-only and the gap can never be filled in place.
//
// So the repair happens on the way out: missing results are synthesized, and
// tool messages answering nothing are dropped. Nothing on disk changes — the
// thread stays the honest record of what happened.
func RepairToolCalls(msgs []ports.ChatMessage) []ports.ChatMessage {
	out := make([]ports.ChatMessage, 0, len(msgs))
	for i := 0; i < len(msgs); i++ {
		m := msgs[i]
		if m.Role == ports.RoleTool {
			// A tool message here answers no preceding call: the loop below
			// consumes every legitimate one.
			continue
		}
		out = append(out, m)
		if len(m.ToolCalls) == 0 {
			continue
		}

		wanted := make(map[string]bool, len(m.ToolCalls))
		for _, call := range m.ToolCalls {
			wanted[call.ID] = true
		}
		answered := make(map[string]bool, len(m.ToolCalls))
		for i+1 < len(msgs) && msgs[i+1].Role == ports.RoleTool {
			i++
			id := msgs[i].ToolCallID
			// One result per call: the API rejects a second answer to a call
			// as surely as it rejects a missing one.
			if !wanted[id] || answered[id] {
				continue
			}
			answered[id] = true
			out = append(out, msgs[i])
		}
		for _, call := range m.ToolCalls {
			if !answered[call.ID] {
				out = append(out, ports.ChatMessage{
					Role:       ports.RoleTool,
					Content:    UnansweredToolCallResult,
					ToolCallID: call.ID,
				})
			}
		}
	}
	return out
}
