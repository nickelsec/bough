// Package pi reads the history of the pi coding agent.
//
// Pi writes one JSONL file per session under
// ~/.pi/agent/sessions/--<cwd>--/<timestamp>_<id>.jsonl. The first line is a
// header naming the working directory, and every line after it is an entry
// with an id and the id of its parent, so a session is a tree rather than a
// list: going back to an earlier point and trying again adds a second child to
// that point instead of starting a new file.
//
// Everything here was read off Pi's own source, version 0.87.1, rather than
// guessed from a sample. The files each piece comes from are named where it is
// used, so a change to the format can be traced back to the line that assumed
// the old one.
package pi

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent/shell"
)

// Entry is one line of a Pi session file.
//
// One struct covers every entry type, the header included, because the types
// share most of their fields and a line has to be parsed before its type is
// known. Fields a type does not use stay empty. See SessionEntry and
// SessionHeader in packages/coding-agent/src/core/session-manager.ts.
type Entry struct {
	Type string `json:"type"`

	// ID and ParentID place the entry in the session's tree. A version 1
	// file has neither, and its entries run in file order.
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`

	// Timestamp is ISO 8601 on every entry, unlike the milliseconds inside a
	// message.
	Timestamp string `json:"timestamp"`

	// Header fields. Version is missing on version 1 files.
	Version       int    `json:"version"`
	Cwd           string `json:"cwd"`
	ParentSession string `json:"parentSession"`

	// Message is set on "message" entries.
	Message *Message `json:"message"`

	// Provider and ModelID are set on "model_change".
	Provider string `json:"provider"`
	ModelID  string `json:"modelId"`

	// Model and Usage are set on "usage" entries, which record model work
	// that is not a reply, cache warming for instance. Usage also appears on
	// "compaction" and "branch_summary" entries for the summary they paid for.
	Model string `json:"model"`
	Usage *Usage `json:"usage"`

	// Name is set on "session_info", from /name.
	Name string `json:"name"`
}

// Time is when the entry was written, in the reader's own zone.
func (e *Entry) Time() time.Time { return shell.When(e.Timestamp) }

// Message is what a "message" entry carries. As with Entry, one struct covers
// every role. See packages/ai/src/types.ts and
// packages/coding-agent/src/core/messages.ts.
type Message struct {
	Role string `json:"role"`

	// Content is a string or a list of blocks for a user message, and always a
	// list for an assistant reply or a tool result.
	Content json.RawMessage `json:"content"`

	// Assistant fields.
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	ResponseModel string `json:"responseModel"`
	Usage         *Usage `json:"usage"`
	StopReason    string `json:"stopReason"`

	// Tool result fields. Usage is also set on a tool result that did model
	// work of its own.
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Details    json.RawMessage `json:"details"`
	IsError    bool            `json:"isError"`

	// Fields of a command the person ran themselves with "!".
	Command   string `json:"command"`
	Output    string `json:"output"`
	ExitCode  *count `json:"exitCode"`
	Cancelled bool   `json:"cancelled"`
}

// Usage is what one piece of model work was charged for.
//
// The four counts do not overlap: Input excludes what was read from the cache,
// and Reasoning is already inside Output, so it is not read at all.
// CacheWrite1h is the part of CacheWrite held for an hour. The cost Pi worked
// out is kept only so tests can hold bough's own figure against it.
type Usage struct {
	Input        count `json:"input"`
	Output       count `json:"output"`
	CacheRead    count `json:"cacheRead"`
	CacheWrite   count `json:"cacheWrite"`
	CacheWrite1h count `json:"cacheWrite1h"`
	Cost         cost  `json:"cost"`
}

// cost is what Pi worked out a piece of usage cost, in dollars.
//
// A reply's usage carries it as an object with a total. The sub-agent
// extension's summed usage carries it as a bare number. Expecting only the
// object failed the whole result on the other shape, and a sub-agent's work
// disappeared with it.
type cost struct {
	Total float64
}

func (c *cost) UnmarshalJSON(b []byte) error {
	var n float64
	if json.Unmarshal(b, &n) == nil {
		c.Total = n
		return nil
	}
	var o struct {
		Total float64 `json:"total"`
	}
	_ = json.Unmarshal(b, &o)
	c.Total = o.Total
	return nil
}

// Block is one piece of a message's content.
type Block struct {
	Type string `json:"type"` // "text", "image", "thinking", "toolCall"
	Text string `json:"text"`

	// Tool call fields.
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// count is a number that tolerates being written as a float.
//
// Pi writes whole numbers, but it is JavaScript, where every number is a
// float, and a count that one day arrives as 1234.0 would fail to parse as an
// int and take the whole line with it, and the reply's usage would vanish
// from the total without anything saying so.
type count int

func (c *count) UnmarshalJSON(b []byte) error {
	// null, or something that is not a number, counts as nothing rather than
	// spoiling the entry around it.
	*c = 0
	var f float64
	if json.Unmarshal(b, &f) == nil {
		*c = count(f)
	}
	return nil
}

// blocks reads a message's content as a list, whichever shape it was stored
// in. A bare string becomes a single text block.
func blocks(raw json.RawMessage) []Block {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return []Block{{Type: "text", Text: s}}
	}
	var out []Block
	_ = json.Unmarshal(raw, &out)
	return out
}

// text joins the text blocks of a message, leaving everything else out.
func text(raw json.RawMessage) string {
	var parts []string
	for _, b := range blocks(raw) {
		if b.Type == "text" && b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// hasImage reports whether a message carries a picture.
func hasImage(raw json.RawMessage) bool {
	for _, b := range blocks(raw) {
		if b.Type == "image" {
			return true
		}
	}
	return false
}
