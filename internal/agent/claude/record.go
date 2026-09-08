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
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
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

	// IsMeta marks a record the harness wrote rather than the user. It is set
	// on things like the caveat that precedes local command output, which
	// otherwise look like ordinary typed prompts.
	IsMeta bool `json:"isMeta"`

	Message *Message `json:"message"`

	// ToolUseResult carries what a tool returned. Only the git operation is
	// read from it, since that is the one part of a result that says something
	// about the work rather than about the tool.
	ToolUseResult *ToolUseResult `json:"toolUseResult"`
}

// ToolUseResult is the outcome of a tool call, filed on the record after it.
type ToolUseResult struct {
	GitOperation *GitOperation `json:"gitOperation"`

	// StructuredPatch is the diff an edit produced. It says how much changed,
	// which the call on its own does not: a typo and a rewrite are both one
	// call.
	StructuredPatch []Hunk `json:"structuredPatch"`

	// NewString and Content are what a write put in the file. Only a diff
	// carries a patch, so a file created from nothing has none, and these are
	// what is left to measure it by. An edit uses the first, a write the
	// second.
	NewString string `json:"newString"`
	Content   string `json:"content"`
}

// Hunk is one run of changed lines in a patch.
type Hunk struct {
	// Lines are the diff lines, each opening with "+", "-" or a space.
	Lines []string `json:"lines"`
}

// Changed counts the lines this result added or removed.
//
// Both directions count as change: rewriting a line is a removal and an
// addition, and moving a block around a file is work whether or not the totals
// come out even.
func (t *ToolUseResult) Changed() int {
	if t == nil {
		return 0
	}
	n := 0
	for _, h := range t.StructuredPatch {
		for _, l := range h.Lines {
			if len(l) > 0 && (l[0] == '+' || l[0] == '-') {
				n++
			}
		}
	}
	if n > 0 {
		return n
	}
	// No patch. A write to a new file has nothing to diff against, and there
	// are 415 of those against 1,121 patches on the corpus this was fitted to,
	// so counting them as nothing would make creating a file look like no work.
	for _, written := range []string{t.NewString, t.Content} {
		if written != "" {
			return strings.Count(written, "\n") + 1
		}
	}
	return 0
}

// UnmarshalJSON accepts either form of tool result.
//
// Most tools return an object, but plenty return a bare string: 85 of the 340
// lines in the replay fixture do. Treating this as an object only would fail
// the whole line and lose the record, which is how this was found.
func (t *ToolUseResult) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || b[0] != '{' {
		return nil
	}
	type plain ToolUseResult
	return json.Unmarshal(b, (*plain)(t))
}

// GitOperation is a git action Claude Code recognised itself performing.
//
// Do not rely on this being present. Claude Code fills it in by reading what
// git printed, so a commit made with -q leaves it empty: across this corpus 53
// of 57 ordinary commits carry it and none of the 88 quiet ones do. The command
// that ran is the dependable signal; this is where a hash comes from when there
// is one.
type GitOperation struct {
	Commit *Commit `json:"commit"`
}

// Commit is a commit the agent made.
//
// Note what this does not cover: a commit the person typed themselves in a
// terminal never reaches the transcript, so this records what the agent
// committed, not everything that was committed.
type Commit struct {
	SHA    string `json:"sha"`
	Kind   string `json:"kind"`
	Branch string `json:"branch"`
}

// Message is the model-facing part of a record.
type Message struct {
	Role    string  `json:"role"`
	Content Content `json:"content"`

	// ID identifies the reply itself, and several records can carry the same
	// one. Deduplicating records by uuid is not enough for anything counted
	// off usage: a single reply is written under more than one uuid while
	// keeping one id, and 1,533 of 2,236 ids in one project did exactly that.
	// Summing per record would count those replies three times over.
	ID string `json:"id"`

	// Model is which model answered. Only assistant records carry it, and a
	// harness placeholder of "<synthetic>" turns up a handful of times per
	// project, which is not a model anyone chose.
	Model string `json:"model"`

	// Usage is what the reply was charged for.
	Usage *Usage `json:"usage"`
}

// Usage is the token count for one reply.
//
// The nested cache_creation breakdown is deliberately not read. It reconciles
// exactly with the flat field, so taking both would risk counting the same
// tokens twice for nothing gained.
type Usage struct {
	Input      int `json:"input_tokens"`
	Output     int `json:"output_tokens"`
	CacheRead  int `json:"cache_read_input_tokens"`
	CacheWrite int `json:"cache_creation_input_tokens"`
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

	// ID identifies a tool call, and ToolUseID on a later result points back to
	// it. Pairing the two is how a command is matched with what it did.
	ID        string `json:"id"`
	ToolUseID string `json:"tool_use_id"`

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

// Commit returns the commit this record reports, or nil if it reports none.
//
// A commit with no hash is not a commit worth recording, since the hash is the
// only part a reader can check against the repository.
func (r *Record) Commit() *agent.Commit {
	if r.ToolUseResult == nil || r.ToolUseResult.GitOperation == nil {
		return nil
	}
	c := r.ToolUseResult.GitOperation.Commit
	if c == nil || c.SHA == "" {
		return nil
	}
	return &agent.Commit{SHA: c.SHA, Kind: c.Kind, Branch: c.Branch}
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
//
// promptId is deliberately not required here. It is a good marker and it is
// used elsewhere to tell one submission from the next, but Claude Code only
// began writing it partway through its life, so requiring it silently discards
// every transcript older than that. One reader measured a project of 238
// sessions drawing as 20 prompts because only two of them were recent enough
// to carry the field; the same records read without it hold 3,282. The absence
// of the field says something about the version that wrote the file, not about
// whether a person typed anything.
//
// isMeta does some of the work promptId was doing. The harness sets it on
// records it wrote itself, which is version independent in a way promptId is
// not.
func (r *Record) IsHumanPrompt() bool {
	if r.Type != "user" || r.IsSidechain || r.IsMeta || r.Message == nil {
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
