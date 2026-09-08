// Package antigravity reads Google Antigravity (AGY CLI / IDE) session history.
//
// Antigravity stores conversations under ~/.gemini/antigravity-cli/brain/<conv-id>/,
// with transcripts logged in .system_generated/logs/transcript.jsonl.
package antigravity

import (
	"encoding/json"
	"regexp"
	"strings"
	"time"
)

// Record is one line of an Antigravity transcript.
type Record struct {
	StepIndex int        `json:"step_index"`
	Source    string     `json:"source"` // "USER_EXPLICIT", "MODEL", "SYSTEM"
	Type      string     `json:"type"`   // "USER_INPUT", "PLANNER_RESPONSE", "GENERIC"
	Status    string     `json:"status"` // "DONE", "ERROR", "RUNNING"
	CreatedAt string     `json:"created_at"`
	Content   string     `json:"content"`
	Thinking  string     `json:"thinking"`
	ToolCalls []ToolCall `json:"tool_calls"`
}

// ToolCall represents a single tool invocation within a PLANNER_RESPONSE step.
type ToolCall struct {
	Name string            `json:"name"`
	Args map[string]string `json:"args"`
}

// CleanArg removes surrounding quotes and unescapes JSON strings when present.
func CleanArg(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		var unquoted string
		if err := json.Unmarshal([]byte(v), &unquoted); err == nil {
			return unquoted
		}
		return v[1 : len(v)-1]
	}
	return v
}

// Time parses the timestamp on the record.
func (r *Record) Time() time.Time {
	if r.CreatedAt == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, r.CreatedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

var userRequestRegex = regexp.MustCompile(`(?s)<USER_REQUEST>\s*(.*?)\s*</USER_REQUEST>`)

// IsHumanPrompt reports whether this record is a user request.
func (r *Record) IsHumanPrompt() bool {
	if r.Type != "USER_INPUT" || r.Source != "USER_EXPLICIT" {
		return false
	}
	if strings.Contains(r.Content, "<SYSTEM_MESSAGE>") && !strings.Contains(r.Content, "<USER_REQUEST>") {
		return false
	}
	return len(strings.TrimSpace(r.PromptText())) > 0
}

// PromptText returns the human request text, stripping harness wrappers like <USER_REQUEST>.
func (r *Record) PromptText() string {
	if m := userRequestRegex.FindStringSubmatch(r.Content); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	// Filter metadata wrappers if present
	content := r.Content
	if idx := strings.Index(content, "<ADDITIONAL_METADATA>"); idx != -1 {
		content = content[:idx]
	}
	if idx := strings.Index(content, "<USER_SETTINGS_CHANGE>"); idx != -1 {
		content = content[:idx]
	}
	return strings.TrimSpace(content)
}
