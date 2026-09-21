// Package agent defines the boundary between bough and the coding agents whose
// history it reads.
//
// Agents store their history in very different ways. Claude Code writes one
// JSONL file per session. Cursor keeps chat in a SQLite database shared by a
// whole workspace. Others will differ again. Nothing above this package is
// allowed to know which of those it is looking at, so a Source hands back
// normalised Turn values and keeps its own storage quirks to itself.
//
// The rule that keeps this honest: internal/segment and internal/rollup must
// never import an agent implementation. They see Turn and nothing else.
package agent

import (
	"path"
	"regexp"
	"strings"
	"time"
)

// Source is one coding agent that bough can read.
type Source interface {
	// Name identifies the agent, for example "claude-code".
	Name() string

	// Detect reports the projects this agent has history for on this machine.
	// A source that is not installed returns no projects and no error.
	Detect() ([]Project, error)

	// Sessions reads every session belonging to a project. A session that
	// cannot be read is reported through the error return without stopping
	// the ones that can, since partial history is still worth showing.
	Sessions(Project) ([]Session, error)
}

// Project is a codebase an agent has worked on.
type Project struct {
	// Name is what the user would call this project, usually the directory name.
	Name string

	// Path is the working directory, recovered from the session records rather
	// than from any directory name the agent may have mangled.
	Path string

	// Source names the agent this came from, as that agent's package spells it.
	Source string

	// Ref locates the project inside the agent's own storage. Its meaning is
	// private to the Source that produced it.
	Ref string

	// LastWorked is when the history was last added to. It comes from the
	// files rather than from their contents, so listing projects stays cheap
	// even on a large history.
	LastWorked time.Time

	// Bytes is roughly how much history there is, again from the files rather
	// than their contents. It says which projects are substantial, not how
	// many prompts they hold.
	Bytes int64
}

// Session is one continuous stretch of work as the agent recorded it.
//
// Do not mistake this for a unit of work. On Claude Code a single session can
// run for nine days and cover a dozen unrelated things, which is the whole
// reason bough has to segment from the inside.
type Session struct {
	ID    string
	Title string // the agent's own label for the session, when it has one
	Turns []Turn

	// ParentID names the session that delegated this work, empty when a person
	// started it.
	//
	// An agent that spawns another gets a session of its own, because the
	// sub-agent has its own prompts and its own token spend and folding those
	// into the parent would hide work that did happen. But it is not a separate
	// stretch of work: it runs inside the turn that asked for it, and drawing it
	// alongside the parent says the person started two things when they started
	// one.
	ParentID string
}

// Known describes one agent bough can read.
//
// Everything a reader or a flag needs to say about an agent lives here, in one
// row per agent. It used to be spelled out in seven places: two Name methods,
// two Source literals, a Display switch, a copy of that switch in the page, the
// --agent flag, and the wording of the "no history found" error. Adding an
// agent meant finding all seven, and the page's copy had already drifted from
// the switch it was copied from.
type Known struct {
	// Source is what the agent's own package calls itself, as it appears on a
	// Project.
	Source string

	// Display is the name a person would recognise. "claude-code" is the name
	// of a source; "Claude Code" is the name of a tool.
	Display string

	// Flag are the words --agent accepts for this one.
	Flag []string

	// Where is the directory its history lives in, for saying where bough
	// looked when it found nothing.
	Where string
}

// Agents is every agent bough reads, in the order they were added.
var Agents = []Known{
	{
		Source:  "claude-code",
		Display: "Claude Code",
		Flag:    []string{"claude", "claude-code"},
		Where:   "~/.claude/projects",
	},
	{
		Source:  "codex",
		Display: "Codex",
		Flag:    []string{"codex"},
		Where:   "~/.codex/sessions",
	},
}

// Lookup finds an agent by its source name.
func Lookup(source string) (Known, bool) {
	for _, k := range Agents {
		if k.Source == source {
			return k, true
		}
	}
	return Known{}, false
}

// ByFlag finds an agent by a word --agent accepts.
func ByFlag(word string) (Known, bool) {
	for _, k := range Agents {
		for _, f := range k.Flag {
			if f == word {
				return k, true
			}
		}
	}
	return Known{}, false
}

// Display names an agent for a person to read.
//
// Both views ask here rather than spelling it themselves, so the terminal and
// the page cannot drift apart. The page is handed this table rather than
// holding a copy of it.
//
// An agent nobody has named yet comes back as it was given, which is better
// than an empty column.
func Display(source string) string {
	if k, ok := Lookup(source); ok {
		return k.Display
	}
	return source
}

// Delegation is a unit of work the agent handed to a sub-agent.
type Delegation struct {
	// Kind is the sort of sub-agent, for example "Explore" or "Plan". Empty
	// when the agent does not name a type.
	Kind string

	// Name is what this particular piece of handed-off work was called, for
	// example "pixel_art". Empty when the agent does not name it.
	//
	// Separate from Kind because they are separate facts and were being put in
	// the same field. Claude names the type of sub-agent and never the task;
	// Codex names the task and often not the type. Reading both out of one
	// string meant every consumer had to know which agent it came from to know
	// what the word in front of it meant.
	Name string

	// Description is what the sub-agent was asked to do, in the words used at
	// the time. Empty when the brief is unreadable: Codex encrypts it.
	Description string
}

// Says returns the best single phrase for a delegation, for a reader who has
// room for one.
//
// The brief is what somebody asked for, so it wins. Failing that the task's
// own name says what the work was, and the type of sub-agent says only who did
// it, which is the least informative of the three. Printing Kind alone left a
// bare "handed off:" with nothing after it on agents that do not set it.
func (d Delegation) Says() string {
	switch {
	case d.Description != "":
		return d.Description
	case d.Name != "":
		return d.Name
	default:
		return d.Kind
	}
}

// Commit is a commit the agent made while working on a turn.
//
// This is the only thing in the graph that is not inferred. Everything else,
// where a task starts and ends and how hard it looked, comes from heuristics.
// A commit either exists in the repository or it does not, which makes it the
// one claim a reader can check.
//
// It is also incomplete by nature: a commit the person typed themselves never
// appears in any agent's history, so this says what the agent committed and
// nothing about the rest.
type Commit struct {
	// SHA is the abbreviated hash the agent recorded, when it recorded one.
	// Claude Code reads the hash back out of what git printed, so a commit made
	// quietly has none until the repository is consulted.
	SHA string

	// Kind is what happened, for example "committed" or "amended".
	Kind string

	// Branch is where it landed, when the agent recorded one.
	Branch string

	// At is when the commit call returned, which is within a second or two of
	// the commit itself. It is what lets a commit with no hash be matched
	// against the repository.
	At time.Time

	// Dir is where the commit was made, when the command moved somewhere first.
	// Empty means the project's own directory.
	//
	// A session about one project often commits in another, a tool and its
	// website being worked on together for instance. Those commits are real but
	// they belong to that other repository, and this is what lets them be told
	// apart.
	Dir string

	// Subject, Added and Removed come from the repository rather than the
	// transcript, and are empty when it could not be read. The transcript knows
	// a commit happened; only the repository knows how big it was.
	Subject string
	Added   int
	Removed int
}

// Tokens is what a stretch of work cost, in the four counts the transcript
// keeps.
//
// The interesting one is CacheRead, and it is interesting because of its size.
// The model has no memory between messages, so every reply re-reads the whole
// conversation so far along with the files and the instructions. Caching makes
// each re-read cheap and it is paid every turn, which on the corpus this was
// built against came to around 596 times the output and most of the bill.
type Tokens struct {
	// Input is text sent fresh, uncached. It is close to nothing in practice.
	Input int

	// Output is what the model wrote. The part everyone pictures, and about a
	// tenth of the cost.
	Output int

	// CacheRead is re-reading what was already sent. Paid every turn.
	CacheRead int

	// CacheWrite is storing context so it can be re-read cheaply. Paid once.
	CacheWrite int

	// CacheWriteHour is the part of CacheWrite held for an hour rather than
	// five minutes, and is never larger than it.
	//
	// It is not a fifth count. It is a slice of the fourth, carried because
	// the two are charged differently: an hour costs about 60% more to store.
	// On the history this was checked against, 96% of cache writes were
	// hourly, so treating them all as the cheaper kind put the bill 4.7%
	// under the published rates. Total deliberately leaves it out, since
	// adding it would count those tokens twice.
	CacheWriteHour int
}

// Add sums another set of counts into this one.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
	t.CacheWriteHour += o.CacheWriteHour
}

// Total is every token the work was charged for.
func (t *Tokens) Total() int {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite
}

// Charge credits a model with what it was charged for, making the map if it is
// not there yet.
//
// The making is the point. Every caller holds a map that is allocated lazily,
// so every caller had the same "if nil" preamble, and one place that skipped
// it dropped a sub-agent's models on the floor without saying so.
func Charge(m *map[string]Tokens, model string, t Tokens) {
	if model == "" {
		return
	}
	if *m == nil {
		*m = map[string]Tokens{}
	}
	was := (*m)[model]
	was.Add(t)
	(*m)[model] = was
}

// Merge sums one per-model set into another.
func Merge(dst *map[string]Tokens, src map[string]Tokens) {
	for model, t := range src {
		Charge(dst, model, t)
	}
}

// Turn is one human prompt and everything the agent did in response.
//
// This is the unit every later stage works from. Anything agent-specific has
// already been resolved by the time a Turn exists.
type Turn struct {
	At   time.Time
	Text string // what the human typed, empty when nobody typed anything

	// TaskName is what a piece of handed-off work was called, when this turn
	// is a sub-agent's rather than a person's.
	//
	// A sub-agent's rollout has no prompt in it: nobody typed anything, the
	// work arrived as an instruction from another agent. That used to be
	// written into Text, first as the task's name and then as the literal
	// "delegated task" when there was no name, which made a field documented
	// as the reader's own words hold something no reader ever wrote. A turn
	// with no prompt now says so by leaving Text empty, and anything with a
	// line to fill asks Says for the best phrase available.
	TaskName string

	// Tools counts calls by tool name.
	Tools map[string]int

	// Files counts every file the agent touched, keyed by normalised path.
	Files map[string]int

	// Edits counts only files the agent changed, a subset of Files.
	Edits map[string]int

	// Lines counts how much changed in each of those files. Edits counts
	// calls, which says a file was touched; this says whether that was a typo
	// or a rewrite. On the corpus this was fitted to, 246 edits changed two
	// lines or fewer and 75 changed over a hundred.
	Lines map[string]int

	// Errors is how many tool calls came back as failures.
	Errors int

	// Tokens is what answering this turn was charged for.
	Tokens Tokens

	// Models is what each model was charged for, keyed by model name. A
	// project usually has one, but a model changed partway through is worth
	// being able to say.
	//
	// The four counts are kept per model rather than a single output figure
	// because the four are priced at wildly different rates: re-reading the
	// cache costs about a tenth of fresh input and writing it about a quarter
	// more, so a blended rate cannot produce a bill. The split was always
	// there to be kept, sitting beside the model name on the same record, and
	// throwing it away was what made the cost of a piece of work unanswerable.
	//
	// These sum to Tokens. A model bough has no price for still appears here,
	// because what it was charged for is a fact about the work either way.
	Models map[string]Tokens

	// Delegated is the sub-agent work this turn started. These are the one place
	// the record holds real branching, and each carries a description written at
	// the time, which makes it a better label than anything inferred later.
	Delegated []Delegation

	// Committed is what the agent committed during this turn, in order.
	Committed []Commit

	// SegmentHint marks a boundary the agent itself recorded, such as a context
	// compaction. Free evidence, worth more than anything we infer.
	SegmentHint bool
}

// Says returns the best phrase describing a turn, for somewhere with room for
// one line.
//
// A prompt is the reader's own words and always wins. A sub-agent's turn has
// none, so the name of the task it was given stands in. A turn with neither
// returns empty rather than a stand-in nobody wrote.
func (t Turn) Says() string {
	if t.Text != "" {
		return t.Text
	}
	return t.TaskName
}

// NormalisePath tidies a path without deciding what kind of path it is.
//
// Separators are settled and the path is cleaned, and that is all. A path that
// names a Windows drive outright, as "D:/Work", is folded to "d:/work", since
// Windows does not distinguish those and the drive letter's case changes
// between records in one session.
//
// Nothing else is case folded, and nothing is rewritten. This function used to
// lower-case every path and rewrite any "/x/..." into "x:/...", on the reading
// that a single letter first component meant a unix style shell had written a
// Windows drive. That is true on Windows and false everywhere else, so
// "/w/app/one.go" was shown to the reader as "w:/app/one.go", and "/home/u/App"
// and "/home/u/app" were merged into one project on a filesystem that says they
// are two, taking the commit hashes of one of them with it.
//
// Deciding that two differently written paths are the same place is a separate
// job, and SamePath does it.
//
// This deliberately does not use path/filepath. A transcript written on
// Windows can be read on any machine, so backslashes have to be understood
// everywhere rather than only where the host happens to use them.
func NormalisePath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.ReplaceAll(p, `\`, "/")
	if m := winDrive.FindStringSubmatch(p); m != nil {
		p = strings.ToLower(m[1]) + ":/" + m[2]
	}
	return path.Clean(p)
}

// SamePath reports whether two paths name the same place.
//
// One project's commits arrived as "d:/thing", "/d/thing" and with no path at
// all inside a single session, and they are one directory: comparing them
// without folding said the work happened somewhere else and threw it away.
//
// The "/d/thing" spelling is what a unix style shell writes for a Windows
// drive, but it is also an ordinary unix directory called "d". Nothing in the
// string says which, so rather than guess, both readings are tried here and the
// paths are the same place if any reading agrees. Guessing is what the old
// normaliser did, and it guessed wrong on every unix path.
//
// Case is folded only when a Windows drive is involved, since that is where it
// is known not to matter.
func SamePath(a, b string) bool {
	a, b = NormalisePath(a), NormalisePath(b)
	if a == b {
		return true
	}
	for _, x := range readings(a) {
		for _, y := range readings(b) {
			if x == y {
				return true
			}
		}
	}
	return false
}

// readings lists the ways one written path could be meant. A path naming a
// Windows drive is also folded to lower case, which is why the shell spelling
// has to be turned into the drive spelling rather than the other way about.
func readings(p string) []string {
	out := []string{p}
	if m := shellDrive.FindStringSubmatch(p); m != nil {
		out = append(out, strings.ToLower(m[1])+":/"+strings.ToLower(m[2]))
	}
	if winDrive.MatchString(p) {
		out = append(out, strings.ToLower(p))
	}
	return out
}

// winDrive matches a path that opens with a Windows drive written as such, as
// "D:/work". Unambiguous: no unix path begins this way.
var winDrive = regexp.MustCompile(`^([a-zA-Z]):/(.*)$`)

// shellDrive matches the "/d/some/path" a unix style shell uses for a Windows
// drive. Ambiguous, since it is also an ordinary unix path, so it is only ever
// used to offer a second reading rather than to rewrite anything.
var shellDrive = regexp.MustCompile(`^/([a-zA-Z])/(.*)$`)
