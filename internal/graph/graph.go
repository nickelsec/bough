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
const SchemaVersion = 1

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

	// Sessions is how many separate sittings-with-the-agent this covers. It is
	// reported because it is a fact about the record, not because it maps to
	// anything the user would recognise as a unit of work.
	Sessions int `json:"sessions"`
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
	Text string `json:"text"`

	Edits  int `json:"edits,omitempty"`
	Files  int `json:"files,omitempty"`
	Errors int `json:"errors,omitempty"`

	// Delegated is work handed to a sub-agent, each with the brief written at
	// the time.
	Delegated []Delegation `json:"delegated,omitempty"`
}

// Delegation is a unit of work given to a sub-agent.
type Delegation struct {
	Kind        string `json:"kind,omitempty"`
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
	Churn     int    `json:"churn,omitempty"`
	ChurnFile string `json:"churnFile,omitempty"`

	// TopFiles are the most edited files, most first.
	TopFiles []FileCount `json:"topFiles,omitempty"`

	// Struggle rates how hard the work looked, from 0 to 1. It is a heuristic
	// built from churn and prompt density, and it has not been checked against
	// anyone's memory of their own work, so treat it as a hint.
	Struggle float64 `json:"struggle"`
}

// FileCount is a file and how many times it changed.
type FileCount struct {
	Path  string `json:"path"`
	Edits int    `json:"edits"`
}
