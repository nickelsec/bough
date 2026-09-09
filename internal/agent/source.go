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

import "time"

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

	// Source names the agent this came from.
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

// Delegation is a unit of work the agent handed to a sub-agent.
type Delegation struct {
	// Kind is the sort of sub-agent, for example "Explore" or "Plan".
	Kind string

	// Description is what the sub-agent was asked to do, in the words used at
	// the time.
	Description string
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
}

// Add sums another set of counts into this one.
func (t *Tokens) Add(o Tokens) {
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
}

// Total is every token the work was charged for.
func (t *Tokens) Total() int {
	return t.Input + t.Output + t.CacheRead + t.CacheWrite
}

// Turn is one human prompt and everything the agent did in response.
//
// This is the unit every later stage works from. Anything agent-specific has
// already been resolved by the time a Turn exists.
type Turn struct {
	At   time.Time
	Text string // what the human typed

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

	// Models counts output tokens by model name. A project usually has one,
	// but a model changed partway through is worth being able to say.
	Models map[string]int

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
