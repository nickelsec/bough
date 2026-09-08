// Package codex reads OpenAI Codex CLI session history.
//
// Codex stores conversation transcripts as JSONL rollout files under
// ~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl.
package codex

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Record is one line of a Codex rollout transcript.
type Record struct {
	Timestamp string          `json:"timestamp"`
	Ordinal   int             `json:"ordinal"`
	Type      string          `json:"type"` // "session_meta", "response_item", "event_msg", "turn_context"
	Payload   json.RawMessage `json:"payload"`
}

// SessionMeta holds session initialization metadata.
type SessionMeta struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Cwd       string `json:"cwd"`
	Timestamp string `json:"timestamp"`
	Model     string `json:"model"`
}

// ResponseItem represents messages, tool calls, and tool outputs.
type ResponseItem struct {
	Type      string          `json:"type"` // "message", "function_call", "custom_tool_call", "function_call_output", "custom_tool_call_output", "reasoning"
	ID        string          `json:"id"`
	Role      string          `json:"role"` // "user", "assistant", "developer"
	Name      string          `json:"name"` // "exec", "apply_patch", "exec_command", "shell_command", "spawn_agent"
	Input     string          `json:"input"`
	Arguments json.RawMessage `json:"arguments"`
	Content   []ContentItem   `json:"content"`
	Output    json.RawMessage `json:"output"`
	CallID    string          `json:"call_id"`
}

// ContentItem is one chunk of content in a message or tool output.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// TurnContext carries context including the active model.
type TurnContext struct {
	Model string `json:"model"`
	Cwd   string `json:"cwd"`
}

// TokenCountInfo holds token usage metrics from an event_msg.
type TokenCountInfo struct {
	Type string `json:"type"`
	Info struct {
		LastTokenUsage struct {
			InputTokens       int `json:"input_tokens"`
			CachedInputTokens int `json:"cached_input_tokens"`
			OutputTokens      int `json:"output_tokens"`
		} `json:"last_token_usage"`
	} `json:"info"`
}

// Time parses the timestamp on the record.
func (r *Record) Time() time.Time {
	if r.Timestamp == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, r.Timestamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

var commandTagRegex = regexp.MustCompile(`(?s)<command>\s*(.*?)\s*</command>`)

// IsHumanPrompt reports whether this record represents a human request.
func (r *Record) IsHumanPrompt() bool {
	if r.Type != "response_item" {
		return false
	}
	var item ResponseItem
	if err := json.Unmarshal(r.Payload, &item); err != nil {
		return false
	}
	if item.Type != "message" || item.Role != "user" {
		return false
	}
	text := r.extractUserText(&item)
	return cleanPrompt(text) != ""
}

// PromptText returns the user's prompt text, omitting system injections and context headers.
func (r *Record) PromptText() string {
	var item ResponseItem
	if err := json.Unmarshal(r.Payload, &item); err != nil {
		return ""
	}
	text := r.extractUserText(&item)
	return cleanPrompt(text)
}

func (r *Record) extractUserText(item *ResponseItem) string {
	var sb strings.Builder
	for _, c := range item.Content {
		if c.Type == "input_text" || c.Type == "text" {
			sb.WriteString(c.Text)
		}
	}
	return sb.String()
}

func cleanPrompt(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	if strings.HasPrefix(text, "# AGENTS.md") ||
		strings.HasPrefix(text, "<INSTRUCTIONS>") ||
		strings.HasPrefix(text, "<environment_context>") ||
		strings.HasPrefix(text, "<permissions instructions>") ||
		strings.HasPrefix(text, "Reviewed Codex session id:") ||
		strings.HasPrefix(text, "The Codex agent has requested the following action:") {
		return ""
	}
	if m := commandTagRegex.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return text
}
