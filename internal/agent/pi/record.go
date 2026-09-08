// Package pi reads Pi session history.
//
// Pi keeps session transcripts as JSONL files under ~/.pi/agent/sessions/,
// organised in directories named after the working directory with separators
// replaced by dashes (e.g. --Users-name-project--).
package pi

import (
	"encoding/json"
	"strings"
	"time"
)

// Record is one line of a Pi transcript.
type Record struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	ParentID  string `json:"parentId"`
	Timestamp string `json:"timestamp"`

	// Session fields (type == "session")
	Version int    `json:"version"`
	CWD     string `json:"cwd"`

	// Model change fields (type == "model_change")
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`

	// Message fields (type == "message")
	Message *Message `json:"message"`
}

// Message is the inner message payload of a Pi record.
type Message struct {
	Role    string  `json:"role"` // "user", "assistant", "toolResult"
	Content Content `json:"content"`
	Model   string  `json:"model"`
	Usage   *Usage  `json:"usage"`

	// ToolResult specific fields
	ToolCallID string `json:"toolCallId"`
	ToolName   string `json:"toolName"`
	IsError    bool   `json:"isError"`
}

// Usage captures the token counts for a Pi reply.
type Usage struct {
	Input      int `json:"input"`
	Output     int `json:"output"`
	CacheRead  int `json:"cacheRead"`
	CacheWrite int `json:"cacheWrite"`
}

// Content is either a plain string or a list of blocks.
type Content struct {
	Text   string
	Blocks []Block
}

// UnmarshalJSON accepts either string or array of blocks.
func (c *Content) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &c.Text)
	}
	return json.Unmarshal(b, &c.Blocks)
}

// Block represents a single chunk within a message content list.
type Block struct {
	Type      string          `json:"type"` // "text", "thinking", "toolCall"
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Time parses the timestamp on the record, returning zero time if empty or invalid.
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

// IsHumanPrompt reports whether this record represents a human prompt.
func (r *Record) IsHumanPrompt() bool {
	if r.Type != "message" || r.Message == nil || r.Message.Role != "user" {
		return false
	}
	c := r.Message.Content
	if strings.TrimSpace(c.Text) != "" {
		return true
	}
	for _, b := range c.Blocks {
		if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
			return true
		}
	}
	return false
}

// PromptText returns the text of the prompt.
func (r *Record) PromptText() string {
	if r.Message == nil {
		return ""
	}
	c := r.Message.Content
	if c.Text != "" {
		return strings.TrimSpace(c.Text)
	}
	var sb strings.Builder
	for _, b := range c.Blocks {
		if b.Type == "text" && b.Text != "" {
			if sb.Len() > 0 {
				sb.WriteByte('\n')
			}
			sb.WriteString(b.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}
