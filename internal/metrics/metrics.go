// Package metrics summarises a run of turns.
//
// These are the numbers that make a piece of work legible without reading it:
// how long it took, how much changed, and how hard it was. Anything displaying
// the work needs them, so they are computed once here rather than recomputed by
// every caller.
package metrics

import (
	"sort"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// Summary describes a run of turns.
type Summary struct {
	// Start and End bound the work in time.
	Start, End time.Time

	// Span is the wall clock time from the first turn to the last. It includes
	// the pauses, so it says how long the work was open rather than how long it
	// took.
	Span time.Duration

	// Active is the time spent with turns close together, which is closer to
	// time actually worked.
	Active time.Duration

	// Turns is how many prompts the user sent.
	Turns int

	// Edits is the total number of file changes, including the agent's own
	// bookkeeping.
	Edits int

	// AmbientEdits is how much of that was the agent maintaining its own plan
	// and memory files rather than the user's work.
	AmbientEdits int

	// Files is how many distinct files were touched.
	Files int

	// Errors is how many tool calls failed.
	Errors int

	// Churn is the most times a single file was changed. Repeatedly rewriting
	// one file is the clearest sign of something not going well, and it is more
	// telling than the error count.
	Churn int

	// ChurnFile is the file behind that count.
	ChurnFile string

	// TopFiles are the most edited files, most first.
	TopFiles []FileCount

	// Tools counts calls by tool name.
	Tools map[string]int

	// Delegated is the sub-agent work started during these turns.
	Delegated []agent.Delegation
}

// FileCount is a file and how many times it was changed.
type FileCount struct {
	Path  string
	Edits int
}

// activeGap is how long a pause can be before the time stops counting as work.
// Anything longer is a break rather than thinking time.
const activeGap = 20 * time.Minute

// Summarise reduces a run of turns to the numbers that describe it.
func Summarise(turns []agent.Turn) Summary {
	var s Summary
	if len(turns) == 0 {
		return s
	}

	s.Turns = len(turns)
	s.Start = turns[0].At
	s.End = turns[len(turns)-1].At
	s.Span = s.End.Sub(s.Start)
	s.Tools = map[string]int{}

	edits := map[string]int{}
	files := map[string]bool{}

	for i, t := range turns {
		s.Errors += t.Errors
		s.Delegated = append(s.Delegated, t.Delegated...)

		for name, n := range t.Tools {
			s.Tools[name] += n
		}
		for f := range t.Files {
			files[f] = true
		}
		for f, n := range t.Edits {
			s.Edits += n
			if Ambient(f) {
				s.AmbientEdits += n
				continue
			}
			edits[f] += n
		}

		if i > 0 && !t.At.IsZero() && !turns[i-1].At.IsZero() {
			if gap := t.At.Sub(turns[i-1].At); gap <= activeGap {
				s.Active += gap
			}
		}
	}

	s.Files = len(files)
	for f, n := range edits {
		s.TopFiles = append(s.TopFiles, FileCount{Path: f, Edits: n})
	}
	// Sort by edits, then by path so the order does not wander between runs.
	sort.Slice(s.TopFiles, func(i, j int) bool {
		if s.TopFiles[i].Edits != s.TopFiles[j].Edits {
			return s.TopFiles[i].Edits > s.TopFiles[j].Edits
		}
		return s.TopFiles[i].Path < s.TopFiles[j].Path
	})
	if len(s.TopFiles) > 0 {
		s.Churn = s.TopFiles[0].Edits
		s.ChurnFile = s.TopFiles[0].Path
	}
	return s
}

// Struggle rates how hard a piece of work was, from 0 to 1.
//
// Error counts alone do not find the hard parts. Across the history this was
// built against there were only 489 failed calls in 18,173, and the runs that
// felt hardest often had none at all. What they had instead was the same file
// rewritten over and over, and a lot of prompts in a short time. So churn leads,
// with prompt density behind it and errors as a minor third.
//
// This is a heuristic and it has not been checked against anyone's memory of
// their own work. Treat it as a hint, not a measurement.
func (s Summary) Struggle() float64 {
	if s.Turns == 0 {
		return 0
	}

	// A file rewritten a dozen times is a strong signal; beyond that it is not
	// meaningfully worse.
	churn := ratio(float64(s.Churn), 12)

	// Many prompts against few files means going round in circles. Many prompts
	// across many files is just a big piece of work.
	density := 0.0
	if s.Files > 0 {
		density = ratio(float64(s.Turns)/float64(s.Files), 3)
	}

	errors := ratio(float64(s.Errors)/float64(s.Turns), 1)

	return clamp(0.55*churn + 0.30*density + 0.15*errors)
}

func ratio(v, full float64) float64 {
	if full <= 0 {
		return 0
	}
	return clamp(v / full)
}

func clamp(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}
