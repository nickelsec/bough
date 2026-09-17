// Package graph is the shape of a project's work, ready to serialise.
//
// This is where the core stops. Everything above it, a web view, a terminal
// summary, someone's script, reads this and adds nothing to it. So it holds
// what is true about the work and nothing about how it might be drawn: no
// coordinates, no colours, no sizes, nothing that changes when a window is
// resized.
//
// The test for whether a field belongs here is whether it would still exist if
// there were no interface at all.
package graph

import "time"

// SchemaVersion is bumped when the shape below changes in a way that would
// break a reader. Consumers should check it and refuse politely rather than
// misread newer output.
//
// 2: a delegation's task name moved from "kind" to "name". Both facts were
// being written into one field, and which one it held depended on the agent:
// Claude put the sort of sub-agent there and Codex the name of the task. A
// reader that took "kind" as the task name now finds it empty on Codex.
const SchemaVersion = 2

// Graph is one project's history.
type Graph struct {
	Schema int `json:"schema"`

	// Generated is when this was produced, which matters because history keeps
	// growing and two graphs of the same project will differ.
	Generated time.Time `json:"generated"`

	// Tool is the version of bough that produced this.
	Tool string `json:"tool,omitempty"`

	Project Project `json:"project"`

	// Goals are in the order the work happened.
	Goals []Goal `json:"goals"`

	// Links join goals that returned to the same files. They are not
	// hierarchy; neither end owns the other.
	Links []Link `json:"links,omitempty"`

	// Totals summarise the whole project.
	Totals Stats `json:"totals"`
}

// Project is the codebase the work was done in.
type Project struct {
	Name string `json:"name"`

	// Path is the working directory, recovered from the records rather than
	// from any directory name the agent may have mangled.
	Path string `json:"path"`

	// Agent names where the history came from, for example "claude-code".
	Agent string `json:"agent"`

	// AgentName is the same agent as a person would read it, "Claude Code".
	//
	// Carried rather than worked out by whoever is drawing. The page used to
	// hold its own copy of the switch that does this, with a comment saying it
	// mirrored the Go one, which is the arrangement that lets two things that
	// must agree stop agreeing.
	AgentName string `json:"agentName,omitempty"`

	// Sessions is how many separate sittings-with-the-agent this covers. It is
	// reported because it is a fact about the record, not because it maps to
	// anything the user would recognise as a unit of work.
	Sessions int `json:"sessions"`

	// RepoRead says the project's git history was consulted.
	//
	// It decides what a commit hash means. Read, and a hash is one the
	// repository confirmed: anything it could not find has been cleared. Not
	// read, and every hash is whatever the transcript claimed, unverified.
	// Without this the two are indistinguishable in the output, which matters
	// because "not a git repository", "git is not installed" and "--no-repo"
	// all arrive here looking the same.
	RepoRead bool `json:"repoRead"`
}

// Goal is a stretch of work done in one sitting.
type Goal struct {
	ID string `json:"id"`

	// Label is text somebody already wrote, never anything invented. Empty
	// when there was nothing honest to use.
	Label string `json:"label,omitempty"`

	// Period reads the way a person would say it, for example "3 to 5 Aug".
	Period string `json:"period,omitempty"`

	// Title is the name the agent gave the session this came from, when it
	// had one.
	Title string `json:"title,omitempty"`

	Stats Stats  `json:"stats"`
	Tasks []Task `json:"tasks"`
}

// Task is a run of turns working towards one thing.
type Task struct {
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`

	// Reasons say why this task was treated as separate from the one before
	// it, so a boundary a reader disagrees with can be traced to its cause.
	Reasons []string `json:"reasons,omitempty"`

	Stats Stats  `json:"stats"`
	Turns []Turn `json:"turns"`
}

// Turn is one prompt and what came of it.
type Turn struct {
	At time.Time `json:"at"`

	// Text is what the user typed, in full. This is their own writing and it
	// is the reason to click into anything, so it is not trimmed.
	//
	// A turn nobody typed carries the name of the work instead. A sub-agent's
	// rollout has no prompt in it, and when the parent session is absent there
	// is nothing to fold the turn into, so it stays a turn of its own. Leaving
	// this empty showed it as a blank prompt that could not be read or searched
	// for, even though the transcript named the task.
	Text string `json:"text"`

	Edits  int `json:"edits,omitempty"`
	Files  int `json:"files,omitempty"`
	Errors int `json:"errors,omitempty"`

	// Delegated is work handed to a sub-agent, each with the brief written at
	// the time.
	Delegated []Delegation `json:"delegated,omitempty"`

	// Committed is what this prompt produced. Held per prompt as well as per
	// task so the record can show a commit against the thing that asked for it.
	Committed []Commit `json:"committed,omitempty"`
}

// Delegation is a unit of work given to a sub-agent.
type Delegation struct {
	// Kind is the sort of sub-agent, Name is what the task was called, and
	// either may be absent: agents record one or the other, rarely both.
	Kind        string `json:"kind,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
}

// Link joins two goals that worked on the same files.
type Link struct {
	From string `json:"from"`
	To   string `json:"to"`

	// Files are the shared files, most worked first.
	Files []string `json:"files"`

	// Weight is how much shared work sits behind the link.
	Weight int `json:"weight"`
}

// Stats are the numbers that describe a piece of work.
type Stats struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`

	// SpanMinutes is wall clock time including the pauses. ActiveMinutes
	// leaves out the breaks and is closer to time actually worked.
	SpanMinutes   int `json:"spanMinutes"`
	ActiveMinutes int `json:"activeMinutes"`

	Turns  int `json:"turns"`
	Edits  int `json:"edits"`
	Files  int `json:"files"`
	Errors int `json:"errors"`

	// Churn is the most times one file was rewritten, and ChurnFile is which.
	Churn int `json:"churn,omitempty"`

	// LineChurn is how much of that file changed, which says whether coming
	// back to it meant a typo or a rewrite.
	LineChurn int    `json:"lineChurn,omitempty"`
	ChurnFile string `json:"churnFile,omitempty"`

	// Tokens is what this work was charged for. The cache figure dwarfs the
	// rest: the model re-reads the conversation every turn, which on the
	// history this was built against came to most of the cost.
	Tokens *Tokens `json:"tokens,omitempty"`

	// Models counts output tokens by model, so a project that changed model
	// partway through can say so.
	Models map[string]int `json:"models,omitempty"`

	// TopFiles are the most edited files, most first.
	TopFiles []FileCount `json:"topFiles,omitempty"`

	// Commits are what the agent committed during this work, in order.
	//
	// Everything else here is inferred. This is not: a hash either exists in
	// the repository or it does not. It is also incomplete on purpose, since a
	// commit made by hand in a terminal never reaches an agent's history.
	Commits []Commit `json:"commits,omitempty"`

	// Struggle rates how hard the work looked, from 0 to 1. It is a heuristic
	// built from churn and prompt density, and it has not been checked against
	// anyone's memory of their own work, so treat it as a hint.
	Struggle float64 `json:"struggle"`
}

// Commit is a commit the agent made.
type Commit struct {
	SHA    string `json:"sha"`
	Kind   string `json:"kind,omitempty"`
	Branch string `json:"branch,omitempty"`

	// Subject and the line counts come from the repository. They are absent
	// when it could not be read, or when the commit no longer exists in it.
	Subject string `json:"subject,omitempty"`
	Added   int    `json:"added,omitempty"`
	Removed int    `json:"removed,omitempty"`
}

// Tokens is what a stretch of work was charged for.
type Tokens struct {
	Input      int `json:"input,omitempty"`
	Output     int `json:"output,omitempty"`
	CacheRead  int `json:"cacheRead,omitempty"`
	CacheWrite int `json:"cacheWrite,omitempty"`
}

// FileCount is a file and how many times it changed.
type FileCount struct {
	Path  string `json:"path"`
	Edits int    `json:"edits"`
}
