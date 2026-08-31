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
}

// Delegation is a unit of work the agent handed to a sub-agent.
type Delegation struct {
	// Kind is the sort of sub-agent, for example "Explore" or "Plan".
	Kind string

	// Description is what the sub-agent was asked to do, in the words used at
	// the time.
	Description string
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

	// Errors is how many tool calls came back as failures.
	Errors int

	// Delegated is the sub-agent work this turn started. These are the one place
	// the record holds real branching, and each carries a description written at
	// the time, which makes it a better label than anything inferred later.
	Delegated []Delegation

	// SegmentHint marks a boundary the agent itself recorded, such as a context
	// compaction. Free evidence, worth more than anything we infer.
	SegmentHint bool
}
