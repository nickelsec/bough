package claude

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"

	"github.com/nickelsec/bough/internal/agent"
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

// normalisePath puts a file path into a comparable form.
//
// The same file turns up written several ways across a session, because the
// drive letter changes case between records and separators differ by platform.
// Grouping by path only works once those are settled.
//
// This deliberately does not use path/filepath. A transcript written on
// Windows can be read on any machine, so backslashes have to be understood
// everywhere rather than only where the host happens to use them.
func normalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = path.Clean(strings.ReplaceAll(p, `\`, "/"))
	return strings.ToLower(p)
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

// commitCall matches a git commit in a shell command.
//
// Anchored to the start of a command or to a shell separator, so a commit
// somebody merely mentioned inside an echoed string is not counted. Options
// between git and the subcommand are allowed, since "git -C dir commit" is
// ordinary. "commit-tree" is excluded by requiring a word boundary that is not
// a hyphen.
// Options may sit between git and the subcommand, and an option or its value
// may carry a quoted run with spaces in it. That last part is not a nicety:
// setting an identity inline, as
//
//	git -c user.name="Ada Lovelace" commit -q
//
// is common, and a pattern that stops at the space inside the quotes misses
// the commit entirely. Seven of one project's thirty one were lost that way.
var optionRun = `-\S*(?:"[^"]*"|'[^']*'|\S)*\s+` +
	`(?:(?:"[^"]*"|'[^']*'|[^-\s])(?:"[^"]*"|'[^']*'|\S)*\s+)?`

// A commit can also open the body of a conditional or a loop, where the
// keyword does the separating that a semicolon would elsewhere.
var commitCall = regexp.MustCompile(`(?:^|[|;&(]|&&|\|\||\b(?:then|else|do)\b)\s*(?:cd\s+\S+\s*&&\s*)*` +
	`git\s+(?:` + optionRun + `)*commit(?:\s|$)`)

// heredoc opens a run of text written into a file or piped to a program.
//
// Everything after it is content rather than command, and content mentioning
// git commit is not a commit. Writing a script or a test about committing does
// exactly that: it accounted for seven of one project's forty seven apparent
// commits and two of another's forty nine, which is the whole of that project's
// disagreement with its own git log.
var heredoc = regexp.MustCompile(`<<-?\s*['"]?\w`)

// dryRun marks a commit that reports what it would do and then does nothing.
// It is a rehearsal, and counting it would credit work that never landed.
var dryRun = regexp.MustCompile(`(?:^|\s)--dry-run\b`)

// isCommit reports whether a shell command actually commits.
//
// Every match is considered rather than only the first, because one line can
// hold several git calls and the first is not always the one that lands. A
// rehearsal followed by the real thing is exactly that shape, and stopping at
// the first match would throw the commit away.
func isCommit(cmd string) bool {
	h := heredoc.FindStringIndex(cmd)
	for _, at := range commitCall.FindAllStringIndex(cmd, -1) {
		// A heredoc before it means the match is inside written text, and so
		// is everything after it.
		if h != nil && h[0] < at[0] {
			return false
		}
		// A rehearsal reports what it would do and does nothing. The flag sits
		// after the subcommand, and only as far as the next separator: beyond
		// that it belongs to some other command on the line.
		if dryRun.MatchString(firstCommand(cmd[at[1]:])) {
			continue
		}
		return true
	}
	return false
}

// separator ends one command and begins the next.
var separator = regexp.MustCompile(`[|;&]`)

// firstCommand is the run of text up to the next command separator, which is
// the part an option can belong to.
func firstCommand(s string) string {
	if at := separator.FindStringIndex(s); at != nil {
		return s[:at[0]]
	}
	return s
}

// amendCall marks a commit that rewrites the one before it rather than adding
// to the history.
var amendCall = regexp.MustCompile(`\bgit\s[^|;&]*\s--amend\b`)

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
			if opened[r.PromptID] {
				continue
			}
			opened[r.PromptID] = true
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
				if in.Command != "" && b.ID != "" && isCommit(in.Command) {
					pending[b.ID] = pendingCommit{
						turn:  len(turns) - 1,
						amend: amendCall.MatchString(in.Command),
						dir:   commitDir(in.Command),
					}
				}
				p := normalisePath(in.path())
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
			return normalisePath(g)
		}
	}
	return ""
}
