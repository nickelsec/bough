package pi

import (
	"encoding/json"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
)

// ExtractTurns turns a session's entries into turns, one per prompt.
//
// Every entry is read, in the order it was written, including the ones on
// branches the person later abandoned. Pi builds its context by walking back
// from the newest entry, which is right for deciding what the model sees next
// and wrong for saying what happened: an abandoned attempt was still typed and
// answered, and it was paid for.
//
// cwd is the session's working directory, which relative paths in tool calls
// are resolved against.
func ExtractTurns(entries []*Entry, cwd string) []agent.Turn {
	return extract(entries, cwd, nil)
}

// extract is ExtractTurns knowing which providers in Pi's models.json run
// locally, which is what lets local work be priced at nothing.
func extract(entries []*Entry, cwd string, ends endpoints) []agent.Turn {
	w := &walker{
		cwd:     cwd,
		ends:    ends,
		pending: map[string]shell.PendingCommit{},
		edits:   map[string]pendingEdit{},
	}
	for _, e := range entries {
		w.entry(e)
	}
	return w.turns
}

// walker holds what reading one session needs to remember between entries.
type walker struct {
	cwd   string
	ends  endpoints
	turns []agent.Turn

	// prev is the id of the last entry read, which is what the next one's
	// parent is when the person carried straight on.
	prev string

	// provider and model are the ones in use, for usage that does not name
	// its own model: a compaction's summary, or a tool's nested work.
	provider, model string

	// early is model work done before anyone typed anything, cache warming for
	// instance. It is held for the first turn rather than dropped.
	early       agent.Tokens
	earlyModels map[string]agent.Tokens

	// pending holds commit commands waiting for their result, and edits holds
	// edits waiting for theirs, both keyed by tool call id. A result arrives
	// as its own entry, sometimes after the next prompt.
	pending map[string]shell.PendingCommit
	edits   map[string]pendingEdit
}

// pendingEdit is a change to a file whose size is only known once the tool
// says how it went.
type pendingEdit struct {
	turn int
	file string

	// guess is how many lines the call's own arguments say changed, used when
	// the result carries no patch to count.
	guess int
}

func (w *walker) cur() *agent.Turn {
	if len(w.turns) == 0 {
		return nil
	}
	return &w.turns[len(w.turns)-1]
}

func (w *walker) entry(e *Entry) {
	if e.Type == "session" {
		return
	}

	// An entry whose parent is not the entry before it means the person went
	// back to an earlier point in the tree, with /tree, and carried on from
	// there. What came before and what comes after are different attempts, a
	// boundary Pi recorded rather than one bough has to infer. A second root,
	// which Pi allows, is the same thing.
	if e.ID != "" {
		if w.prev != "" && (e.ParentID == nil || *e.ParentID != w.prev) {
			w.boundary()
		}
		w.prev = e.ID
	}

	switch e.Type {
	case "model_change":
		w.provider, w.model = e.Provider, e.ModelID

	case "usage":
		// Cache warming and the like. Pi counts these in a session's totals and
		// so does bough. An unknown kind is still usage, as Pi's own docs say.
		if e.Usage != nil {
			w.charge(e.Provider, e.Model, e.Usage)
		}

	case "compaction", "branch_summary":
		// Both summarise work that is being left behind, which is as clear a
		// boundary as the record holds. The summary itself was written by the
		// model in use and is charged to it.
		w.boundary()
		if e.Usage != nil {
			w.charge(w.provider, w.model, e.Usage)
		}

	case "message":
		if e.Message != nil {
			w.message(e.Message, e.Time())
		}
	}
}

// boundary marks the turn in progress as the last one before a break, which is
// how the segmenter reads a hint.
func (w *walker) boundary() {
	if c := w.cur(); c != nil {
		c.SegmentHint = true
	}
}

func (w *walker) message(m *Message, at time.Time) {
	switch m.Role {
	case "user":
		w.prompt(m, at)

	case "assistant":
		if m.Provider != "" || m.Model != "" {
			w.provider, w.model = m.Provider, m.Model
		}
		if m.Usage != nil {
			w.charge(m.Provider, chargedModel(m), m.Usage)
		}
		c := w.cur()
		if c == nil {
			return
		}
		for _, b := range blocks(m.Content) {
			if b.Type == "toolCall" {
				w.call(c, b)
			}
		}

	case "toolResult":
		w.result(m, at)

	case "bashExecution":
		w.typedCommand(m, at)
	}
}

// chargedModel is the model a reply was billed as.
//
// Pi prices a reply by the model that was asked for, and bough does the same,
// with the one exception Pi makes too. When Anthropic answers with a fallback
// model, the fallback is what did the work and what Pi charges for. Anywhere
// else the answering model's name is only the provider's own spelling of the
// one requested: OpenRouter writes "vendor/model" without the ":free" on the
// end, so pricing by it would bill free work at the paid rate. See
// packages/ai/src/api/anthropic-messages.ts and openai-completions.ts.
func chargedModel(m *Message) string {
	if m.Provider == "anthropic" && m.ResponseModel != "" {
		return m.ResponseModel
	}
	return m.Model
}

// charge adds one piece of model work to the turn in progress, or holds it for
// the first turn when nobody has typed anything yet.
func (w *walker) charge(provider, model string, u *Usage) {
	t := tokens(u)
	if t == (agent.Tokens{}) {
		// A request that failed before it was answered carries all zero usage.
		// Charging it would put a model with nothing against it on the bill.
		return
	}
	// Work whose model cannot be known is still charged, under a name that
	// says so. Leaving it off the per-model bill would have the total come out
	// short with nothing to say why; charged like this, the cost is refused
	// and the reason is shown.
	name := w.ends.name(provider, model, u.Cost.Total)
	c := w.cur()
	if c == nil {
		w.early.Add(t)
		agent.Charge(&w.earlyModels, name, t)
		return
	}
	c.Tokens.Add(t)
	agent.Charge(&c.Models, name, t)
}

func tokens(u *Usage) agent.Tokens {
	t := agent.Tokens{
		Input:          int(u.Input),
		Output:         int(u.Output),
		CacheRead:      int(u.CacheRead),
		CacheWrite:     int(u.CacheWrite),
		CacheWriteHour: int(u.CacheWrite1h),
	}
	if t.CacheWriteHour > t.CacheWrite {
		t.CacheWriteHour = t.CacheWrite
	}
	return t
}

// prompt opens a turn for something the person sent.
func (w *walker) prompt(m *Message, at time.Time) {
	said := promptText(text(m.Content))
	// A picture with nothing typed beside it is still something the person
	// sent and asked about. It opens a turn, with no words to show for it
	// rather than words nobody wrote.
	if said == "" && !hasImage(m.Content) {
		return
	}
	w.turns = append(w.turns, agent.Turn{
		At:    at,
		Text:  said,
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
		Lines: map[string]int{},
	})
	c := w.cur()
	if w.earlyModels != nil || w.early != (agent.Tokens{}) {
		c.Tokens.Add(w.early)
		agent.Merge(&c.Models, w.earlyModels)
		w.early, w.earlyModels = agent.Tokens{}, nil
	}
}

// skillBlock is what Pi stores when a prompt invokes a skill: the skill's whole
// text wrapped in a tag, then whatever the person typed after the command.
// The pattern is Pi's own, from parseSkillBlock in
// packages/coding-agent/src/core/agent-session.ts.
var skillBlock = regexp.MustCompile(`(?s)^<skill name="([^"]+)" location="[^"]+">\n.*?\n</skill>(?:\n\n(.+))?$`)

// promptText is what the person typed, with anything Pi expanded on the way in
// put back the way it was written.
//
// A skill is the case that matters. "/skill:review this file" is stored as the
// review skill's entire text followed by "this file", and showing that as the
// prompt would label the work with a page of instructions nobody typed.
func promptText(s string) string {
	s = strings.TrimSpace(s)
	if m := skillBlock.FindStringSubmatch(s); m != nil {
		cmd := "/skill:" + m[1]
		if rest := strings.TrimSpace(m[2]); rest != "" {
			cmd += " " + rest
		}
		return cmd
	}
	return s
}

// call reads one tool call. See the tools in
// packages/coding-agent/src/core/tools for each one's arguments.
func (w *walker) call(c *agent.Turn, b Block) {
	c.Tools[b.Name]++
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
		Command string `json:"command"`
		Edits   []struct {
			OldText string `json:"oldText"`
			NewText string `json:"newText"`
		} `json:"edits"`

		// The older edit shape, one replacement at the top level. Pi still
		// accepts it, so older sessions carry it.
		OldText *string `json:"oldText"`
		NewText *string `json:"newText"`

		// The sub-agent extension's three ways of being asked.
		Agent string `json:"agent"`
		Task  string `json:"task"`
		Tasks []struct {
			Agent string `json:"agent"`
			Task  string `json:"task"`
		} `json:"tasks"`
		Chain []struct {
			Agent string `json:"agent"`
			Task  string `json:"task"`
		} `json:"chain"`
	}
	_ = json.Unmarshal(b.Arguments, &args)
	turn := len(w.turns) - 1

	switch b.Name {
	case "read", "grep", "find", "ls":
		if f := w.resolve(args.Path); f != "" {
			c.Files[f]++
		}

	case "write":
		if f := w.resolve(args.Path); f != "" {
			c.Files[f]++
			c.Edits[f]++
			w.edits[b.ID] = pendingEdit{turn: turn, file: f, guess: lineCount(args.Content)}
		}

	case "edit":
		if f := w.resolve(args.Path); f != "" {
			c.Files[f]++
			c.Edits[f]++
			guess := 0
			for _, e := range args.Edits {
				guess += lineCount(e.NewText)
			}
			if args.NewText != nil {
				guess += lineCount(*args.NewText)
			}
			w.edits[b.ID] = pendingEdit{turn: turn, file: f, guess: guess}
		}

	case "bash", "powershell":
		if shell.IsCommit(args.Command) {
			w.pending[b.ID] = shell.PendingCommit{
				Turn:  turn,
				Amend: shell.IsAmend(args.Command),
				Dir:   shell.CommitDir(args.Command),
			}
		}

	case "subagent":
		// The extension takes one task, a list run side by side, or a chain
		// run in order. Each is a piece of work handed off, so each is a
		// delegation of its own. See examples/extensions/subagent/index.ts.
		if args.Task != "" || args.Agent != "" {
			c.Delegated = append(c.Delegated, agent.Delegation{Kind: args.Agent, Description: args.Task})
		}
		for _, t := range args.Tasks {
			c.Delegated = append(c.Delegated, agent.Delegation{Kind: t.Agent, Description: t.Task})
		}
		for _, t := range args.Chain {
			c.Delegated = append(c.Delegated, agent.Delegation{Kind: t.Agent, Description: t.Task})
		}
	}
}

// result reads what a tool call came back with.
func (w *walker) result(m *Message, at time.Time) {
	// Nested model work a tool did for itself. Pi counts it in the session's
	// totals without naming a model, so it goes to the one in use.
	if m.Usage != nil {
		w.charge(w.provider, w.model, m.Usage)
	}

	if c := w.cur(); c != nil && m.IsError {
		c.Errors++
	}

	id := m.ToolCallID
	if e, held := w.edits[id]; held && id != "" {
		delete(w.edits, id)
		// A failed edit changed nothing. The call still counts as an edit, the
		// way the other readers count it, since the attempt was made.
		if !m.IsError && e.turn >= 0 && e.turn < len(w.turns) {
			n, ok := patchLines(m.Details)
			if !ok {
				n = e.guess
			}
			if n > 0 {
				w.turns[e.turn].Lines[e.file] += n
			}
		}
	}

	if p, held := w.pending[id]; held && id != "" {
		delete(w.pending, id)
		// A command that exits non-zero comes back as an error, which is how
		// a commit with nothing staged or a hook that said no is recognised.
		// See the shell tool in packages/coding-agent/src/core/tools/bash.ts.
		if !m.IsError && p.Turn >= 0 && p.Turn < len(w.turns) {
			w.turns[p.Turn].Committed = append(w.turns[p.Turn].Committed, commit(p, text(m.Content), at))
		}
	}

	if m.ToolName == "subagent" {
		w.subagent(m.Details, at)
	}
}

// typedCommand reads a command the person ran themselves, with "!" or "!!".
//
// These are not the agent's tool calls, so they are not counted as tools. A
// commit made this way is still a commit made during the turn, in the history,
// and checkable against the repository, so it is kept.
func (w *walker) typedCommand(m *Message, at time.Time) {
	c := w.cur()
	if c == nil || m.Cancelled || m.ExitCode == nil || *m.ExitCode != 0 {
		return
	}
	if !shell.IsCommit(m.Command) {
		return
	}
	p := shell.PendingCommit{
		Turn:  len(w.turns) - 1,
		Amend: shell.IsAmend(m.Command),
		Dir:   shell.CommitDir(m.Command),
	}
	c.Committed = append(c.Committed, commit(p, m.Output, at))
}

func commit(p shell.PendingCommit, output string, at time.Time) agent.Commit {
	c := agent.Commit{Kind: "committed", At: at, Dir: p.Dir}
	if p.Amend {
		c.Kind = "amended"
	}
	c.Branch, c.SHA = shell.CommitRef(output)
	return c
}

// subagent folds a sub-agent's work into the turn that asked for it.
//
// The extension runs each sub-agent as a separate process that saves no
// session of its own. What it did survives only in the result it hands back,
// which carries every message the sub-agent exchanged. Pi leaves that work out
// of the session's totals. bough counts it, because it did happen and it was
// paid for: its replies go on the bill under their own models, and its tool
// calls count as the turn's.
//
// A result from an older extension may carry only the summed usage and the
// name of the model. That is still counted, against the model alone.
func (w *walker) subagent(details json.RawMessage, at time.Time) {
	var d struct {
		Results []struct {
			Model    string     `json:"model"`
			Messages []*Message `json:"messages"`
			Usage    *Usage     `json:"usage"`
		} `json:"results"`
	}
	if json.Unmarshal(details, &d) != nil {
		return
	}
	// The sub-agent's own calls and results pair with each other, never with
	// the parent's, so they are read with their own pending sets. The model in
	// use is put back afterwards: the sub-agent's model is not the parent's.
	saved := *w
	w.pending = map[string]shell.PendingCommit{}
	w.edits = map[string]pendingEdit{}
	for _, r := range d.Results {
		if len(r.Messages) == 0 {
			if r.Usage != nil {
				w.charge("", r.Model, r.Usage)
			}
			continue
		}
		for _, m := range r.Messages {
			if m == nil || m.Role == "user" {
				// The brief the sub-agent was handed, already recorded as the
				// delegation. It is not a prompt the person typed.
				continue
			}
			w.message(m, at)
		}
	}
	w.provider, w.model = saved.provider, saved.model
	w.pending, w.edits = saved.pending, saved.edits
}

// patchLines counts the lines an edit changed, from the unified patch Pi keeps
// on the result. See generateUnifiedPatch in
// packages/coding-agent/src/core/tools/edit-diff.ts.
//
// Unlike Codex's patches, these do name files with "---" and "+++" headers, and
// they are only ever the first two lines, so those two are skipped and nothing
// else is: removing a SQL comment "-- note" is the patch line "--- note".
func patchLines(details json.RawMessage) (int, bool) {
	var d struct {
		Patch *string `json:"patch"`
	}
	if json.Unmarshal(details, &d) != nil || d.Patch == nil {
		return 0, false
	}
	n := 0
	header := true
	for _, l := range strings.Split(*d.Patch, "\n") {
		if header {
			if strings.HasPrefix(l, "---") || strings.HasPrefix(l, "+++") ||
				strings.HasPrefix(l, "Index:") || strings.HasPrefix(l, "===") {
				continue
			}
			header = false
		}
		if strings.HasPrefix(l, "\\") {
			continue // "\ No newline at end of file"
		}
		if strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-") {
			n++
		}
	}
	return n, true
}

// lineCount is how many lines a piece of written text spans.
func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(strings.TrimSuffix(s, "\n"), "\n") + 1
}

// resolve turns a path from a tool call into the absolute, normalised form the
// rest of bough keys files by.
//
// It follows Pi's own resolveToCwd, in packages/coding-agent/src/utils/paths.ts
// and tools/path-utils.ts: a leading "@" is dropped, odd Unicode spaces become
// plain ones, "~" is the home directory, a file URL is its path, and a Git Bash
// path such as "/d/work" is the Windows drive it names when the session ran on
// Windows. Anything still relative is taken from the session's directory.
func (w *walker) resolve(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = unicodeSpace.ReplaceAllString(p, " ")
	p = strings.TrimPrefix(p, "@")
	if strings.HasPrefix(p, "file://") {
		p = strings.TrimPrefix(p, "file://")
		// "file:///C:/x" leaves "/C:/x", which is a drive path with a slash
		// in front of it.
		if len(p) > 2 && p[0] == '/' && p[2] == ':' {
			p = p[1:]
		}
	}
	windows := winPath.MatchString(w.cwd)
	if windows {
		if m := gitBashDrive.FindStringSubmatch(p); m != nil {
			p = strings.ToUpper(m[1]) + ":/" + m[2]
		}
	}
	if p == "~" || strings.HasPrefix(p, "~/") || (windows && strings.HasPrefix(p, `~\`)) {
		if home, err := os.UserHomeDir(); err == nil {
			p = home + p[1:]
		}
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if !isAbsolute(p) && w.cwd != "" {
		p = path.Join(strings.ReplaceAll(w.cwd, `\`, "/"), p)
	}
	return agent.NormalisePath(p)
}

func isAbsolute(p string) bool {
	return strings.HasPrefix(p, "/") || winPath.MatchString(p)
}

var (
	unicodeSpace = regexp.MustCompile("[\u00A0\u2000-\u200A\u202F\u205F\u3000]")
	winPath      = regexp.MustCompile(`^[a-zA-Z]:[/\\]`)

	// gitBashDrive is Pi's normalizeWindowsShellPath: "/d", "/d/x",
	// "/mnt/d/x" and "/cygdrive/d/x" all name drive D.
	gitBashDrive = regexp.MustCompile(`(?i)^/(?:mnt/|cygdrive/)?([a-z])(?:/(.*))?$`)
)
