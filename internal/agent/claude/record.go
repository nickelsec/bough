// Package claude reads Claude Code session history.
//
// Claude Code writes one JSONL file per session under ~/.claude/projects, in a
// directory named after the working directory with separators replaced by
// dashes. That mangling is lossy, so the real path comes from the cwd field on
// the records instead.
//
// Two properties of the format drive most of the code here. The files are
// append-only with replay, so the same record can appear several times and
// later copies carry more fields than earlier ones. And the schema is loosely
// typed, with several fields arriving as either a string or a structure. See
// docs/format.md for the measurements behind both.
package claude

import (
	"encoding/json"
	"time"
)

// Record is one line of a transcript.
//
// Only the fields bough uses are named. Everything else is ignored on purpose,
// because the format changes between Claude Code releases and unknown fields
// should never be a parse failure.
type Record struct {
	UUID       string `json:"uuid"`
	ParentUUID string `json:"parentUuid"`
	SessionID  string `json:"sessionId"`
	PromptID   string `json:"promptId"`

	Type    string `json:"type"`
	Subtype string `json:"subtype"`

	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Slug      string `json:"slug"`
	AITitle   string `json:"aiTitle"`
	Version   string `json:"version"`

	IsSidechain bool `json:"isSidechain"`

	Message *Message `json:"message"`
}

// Message is the model-facing part of a record.
type Message struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`
}

// Content is either a plain string or a list of typed blocks. Both shapes occur
// in real transcripts, so it decodes whichever arrived and presents one view.
type Content struct {
	Text   string
	Blocks []Block
}

// UnmarshalJSON accepts either form of message content.
func (c *Content) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] == '"' {
		return json.Unmarshal(b, &c.Text)
	}
	return json.Unmarshal(b, &c.Blocks)
}

// Block is one piece of a message: text, a tool call, or a tool result.
type Block struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`

	// IsError marks a tool result that came back as a failure.
	IsError bool `json:"is_error"`
}

// Time parses the record timestamp, returning the zero time if it is missing or
// malformed. A bad timestamp should never stop a session from loading.
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

// IsCompactBoundary reports whether this record marks a context compaction,
// which Claude Code emits when it summarises and drops earlier history. These
// are reliable segment boundaries because the agent itself decided the thread
// had moved on.
func (r *Record) IsCompactBoundary() bool {
	return r.Type == "system" && r.Subtype == "compact_boundary"
}

// IsHumanPrompt reports whether this record is something the user actually
// typed.
//
// The type field is not enough on its own. Claude Code files tool results under
// type "user" as well, and on the corpus this was built against those
// outnumbered real prompts by more than thirteen to one. A genuine prompt
// carries text, either as a plain string or as text blocks, and never consists
// only of tool results.
func (r *Record) IsHumanPrompt() bool {
	if r.Type != "user" || r.IsSidechain || r.PromptID == "" || r.Message == nil {
		return false
	}
	c := r.Message.Content
	if c.Text != "" {
		return true
	}
	for _, b := range c.Blocks {
		if b.Type == "text" && b.Text != "" {
			return true
		}
	}
	return false
}

// PromptText returns what the user typed, joining text blocks when the content
// arrived in block form. Tool results and other non-text blocks are left out.
func (r *Record) PromptText() string {
	if r.Message == nil {
		return ""
	}
	c := r.Message.Content
	if c.Text != "" {
		return c.Text
	}
	var out []byte
	for _, b := range c.Blocks {
		if b.Type != "text" || b.Text == "" {
			continue
		}
		if len(out) > 0 {
			out = append(out, '\n')
		}
		out = append(out, b.Text...)
	}
	return string(out)
}
