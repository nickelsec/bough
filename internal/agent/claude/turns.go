package claude

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
)

// synthetic matches prompts the harness injected rather than the user typing.
//
// These arrive as ordinary user records with real text in them, so they have to
// be recognised by their opening. Counting them as prompts inflates the record
// of what a person actually asked for.
var syntheticPrefixes = []string{
	"<",                               // reminders and wrappers
	"[Request interrupted",            // the user stopping a turn
	"[Image:",                         // pasted screenshots
	"This session is being continued", // resume preamble
	"Caveat: The messages below",      // harness preamble
	"Base directory for this skill",   // skill loading
	"<local-command-stdout>",          // slash command output
	"API Error",                       // transport failures
}

// isSynthetic reports whether prompt text came from the harness.
func isSynthetic(text string) bool {
	t := strings.TrimSpace(text)
	if t == "" {
		return true
	}
	for _, p := range syntheticPrefixes {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}

// toolInput is the subset of tool arguments worth reading. Tools name their
// file argument differently, so several spellings are accepted.
type toolInput struct {
	FilePath     string `json:"file_path"`
	NotebookPath string `json:"notebook_path"`
	Path         string `json:"path"`

	// Sub-agent calls describe the work they were given, which is a label
	// written at the time rather than one inferred afterwards.
	SubagentType string `json:"subagent_type"`
	Description  string `json:"description"`

	// Command is what a shell tool was asked to run.
	Command string `json:"command"`
}

// The shell package reads meaning out of the commands an agent ran, and does
// it the same way for every agent. What counts as a commit does not depend on
// which harness wrote the transcript.

// path returns whichever file argument the tool supplied.
func (t toolInput) path() string {
	switch {
	case t.FilePath != "":
		return t.FilePath
	case t.NotebookPath != "":
		return t.NotebookPath
	default:
		return t.Path
	}
}

// editingTools are the tools that change a file rather than just reading it.
// Repeated edits to one file are the clearest sign of a struggle, so they are
// tracked apart from reads.
// delegatingTools hand work to a sub-agent.
var delegatingTools = map[string]bool{
	"Task":  true,
	"Agent": true,
}

var editingTools = map[string]bool{
	"Edit":         true,
	"Write":        true,
	"NotebookEdit": true,
}

// ExtractTurns walks a session's records and returns one Turn per human prompt,
// with everything the agent did in response attributed to it.
//
// Records arrive in order, so the work belonging to a prompt is simply
// everything between it and the next one. Synthetic prompts are skipped, and
// any activity they would have owned is credited to the prompt before them,
// which is where it belongs since the harness was acting on that request.
func ExtractTurns(recs []*Record) []agent.Turn {
	var turns []agent.Turn
	var cur *agent.Turn

	// Commit commands waiting on their result, keyed by tool call id. A commit
	// only counts once the call comes back without an error, since plenty are
	// refused for having nothing staged.
	pending := map[string]pendingCommit{}

	// Edits waiting on their result, keyed the same way. The call names the
	// file and the result says how much of it changed.
	edited := map[string]editedFile{}

	// Replies already charged for, by their own id rather than the record's.
	counted := map[string]bool{}

	// Prompts already opened, by promptId. One submission can be written as
	// several user records: invoking a skill files its re-invocation notice,
	// and sometimes the skill body itself, as further records carrying the
	// same promptId. Reading each as a new prompt splits one request into
	// several, which on the corpus this was built against added seventeen
	// prompts that nobody typed. The id is what identifies a submission, so
	// only the first record under one starts a turn.
	//
	// A transcript old enough to predate the field leaves every record with an
	// empty id, which is not one submission repeated but no information at
	// all. Collapsing on it would turn a whole session into a single turn, so
	// records without an id are never grouped and each one stands alone.
	opened := map[string]bool{}

	for _, r := range recs {
		if r.IsCompactBoundary() {
			if cur != nil {
				cur.SegmentHint = true
			}
			continue
		}

		if r.IsHumanPrompt() {
			text := r.PromptText()
			if isSynthetic(text) {
				continue
			}
			// Checked after the synthetic test on purpose. A skipped record
			// must not claim the id, or a real prompt filed under the same one
			// would be dropped rather than merely not duplicated.
			if repeated(opened, r.PromptID) {
				continue
			}
			turns = append(turns, agent.Turn{
				At:    r.Time(),
				Text:  strings.TrimSpace(text),
				Tools: map[string]int{},
				Files: map[string]int{},
				Edits: map[string]int{},
				Lines: map[string]int{},
			})
			cur = &turns[len(turns)-1]
			continue
		}

		if cur == nil {
			continue
		}

		if r.Message == nil {
			continue
		}
		// What the reply was charged for belongs to the prompt that asked
		// for it, and it arrives on the assistant records in between.
		creditUsage(cur, r.Message, counted)
		for _, b := range r.Message.Content.Blocks {
			switch b.Type {
			case "tool_use":
				cur.Tools[b.Name]++
				if len(b.Input) == 0 {
					continue
				}
				var in toolInput
				if err := json.Unmarshal(b.Input, &in); err != nil {
					continue
				}
				if delegatingTools[b.Name] && in.Description != "" {
					cur.Delegated = append(cur.Delegated, agent.Delegation{
						Kind:        in.SubagentType,
						Description: in.Description,
					})
				}
				// Hold the commit until its result says whether it worked.
				if in.Command != "" && b.ID != "" && shell.IsCommit(in.Command) {
					pending[b.ID] = pendingCommit{
						turn:  len(turns) - 1,
						amend: shell.IsAmend(in.Command),
						dir:   commitDir(in.Command),
					}
				}
				p := shell.NormalisePath(in.path())
				if p == "" {
					continue
				}
				cur.Files[p]++
				if editingTools[b.Name] {
					cur.Edits[p]++
					// How much changed is on the result rather than the call,
					// so the file is held until that comes back.
					if b.ID != "" {
						edited[b.ID] = editedFile{turn: len(turns) - 1, path: p}
					}
				}
			case "tool_result":
				if b.IsError {
					cur.Errors++
				}
				recordLines(turns, edited, b, r)
				p, held := pending[b.ToolUseID]
				if !held {
					continue
				}
				delete(pending, b.ToolUseID)
				// A refused commit is not a commit. They are common: nothing
				// staged, or a hook that said no.
				if b.IsError || p.turn < 0 || p.turn >= len(turns) {
					continue
				}
				c := agent.Commit{Kind: "committed", At: r.Time(), Dir: p.dir}
				if p.amend {
					c.Kind = "amended"
				}
				// The hash only exists when Claude Code managed to read it back
				// out of git's own output, which a quiet commit denies it.
				if got := r.Commit(); got != nil {
					c.SHA = got.SHA
					c.Branch = got.Branch
					if got.Kind != "" {
						c.Kind = got.Kind
					}
				}
				turns[p.turn].Committed = append(turns[p.turn].Committed, c)
			}
		}
	}
	return turns
}

// repeated reports whether this submission has already opened a turn, marking
// it as seen when it has not.
//
// An empty id means the transcript predates the field rather than that this is
// the same submission again, so those are never grouped: collapsing on the
// empty string would fold a whole old session into one turn.
func repeated(opened map[string]bool, id string) bool {
	if id == "" {
		return false
	}
	if opened[id] {
		return true
	}
	opened[id] = true
	return false
}

// creditUsage adds a reply's token counts to the turn it answered.
//
// "<synthetic>" is skipped: it is a harness placeholder rather than a model
// anyone chose, and it turns up a handful of times in each project.
func creditUsage(cur *agent.Turn, m *Message, counted map[string]bool) {
	if m.Usage == nil {
		return
	}
	// One reply, several records. Charging per record triples the bill.
	if m.ID != "" {
		if counted[m.ID] {
			return
		}
		counted[m.ID] = true
	}
	cur.Tokens.Add(agent.Tokens{
		Input:      m.Usage.Input,
		Output:     m.Usage.Output,
		CacheRead:  m.Usage.CacheRead,
		CacheWrite: m.Usage.CacheWrite,
	})
	if m.Model == "" || m.Model == "<synthetic>" {
		return
	}
	if cur.Models == nil {
		cur.Models = map[string]int{}
	}
	cur.Models[m.Model] += m.Usage.Output
}

// recordLines credits an edit's size to the turn that made it.
//
// The call names the file and the result says how much changed, so the two
// have to be paired the way commits are.
func recordLines(turns []agent.Turn, edited map[string]editedFile, b Block, r *Record) {
	e, held := edited[b.ToolUseID]
	if !held {
		return
	}
	delete(edited, b.ToolUseID)
	// A refused edit changed nothing.
	if b.IsError || e.turn < 0 || e.turn >= len(turns) {
		return
	}
	if n := r.ToolUseResult.Changed(); n > 0 {
		turns[e.turn].Lines[e.path] += n
	}
}

// editedFile is an edit waiting to hear how much it changed.
type editedFile struct {
	turn int
	path string
}

// pendingCommit is a commit command waiting to hear whether it worked.
type pendingCommit struct {
	turn  int
	amend bool
	dir   string
}

// leadingCD is a command that moves somewhere before doing anything else.
//
// A session about one project regularly commits in another: working on a tool
// and its website together, say. Those commits are real, but they are not this
// project's, and claiming them would have this project's diagram showing work
// that happened somewhere else.
var leadingCD = regexp.MustCompile(`^\s*cd\s+(?:"([^"]*)"|'([^']*)'|([^\s;&|]+))`)

// commitDir is where a command committed, or "" for the project's own
// directory, which is where a command that does not move runs.
func commitDir(cmd string) string {
	m := leadingCD.FindStringSubmatch(cmd)
	if m == nil {
		return ""
	}
	for _, g := range m[1:] {
		if g != "" {
			return shell.NormalisePath(g)
		}
	}
	return ""
}
