// Package rollup groups tasks into goals.
//
// A goal is a stretch of work a person would name in one phrase: the styling
// day, the release, the week spent on the parser. It sits above tasks and below
// the project.
//
// The obvious approach is to group tasks that look alike, by shared files or
// shared vocabulary. That was tried against real history and it does not work.
// Even a task's best match anywhere in its own session sits near noise: on one
// session 22 of 22 tasks had no file overlap above 0.10 with any other task,
// and the sessions that did show file signal showed no word signal, and the
// reverse. There is no threshold that holds across projects, because the two
// signals disagree about which projects they work on.
//
// What does hold is far simpler. People work in sittings. They stop for the
// night, and when they come back they are usually doing something else. Sitting
// boundaries are visible in the timestamps, need no tuning, and cannot drift
// between one project and the next. See docs/tuning.md.
package rollup

import (
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/segment"
)

// Goal is a stretch of tasks worked in one sitting.
type Goal struct {
	Tasks []segment.Task
}

// Options tune where sittings are divided.
type Options struct {
	// Break is how long a pause has to be before the work either side of it
	// counts as separate. The default is set to catch a night's sleep while
	// leaving a lunch break alone.
	Break time.Duration
}

// DefaultOptions are the fitted defaults.
func DefaultOptions() Options {
	return Options{Break: 6 * time.Hour}
}

// Group collects tasks into goals.
//
// Goals are contiguous. Returning to something days later starts a new goal
// rather than reopening the old one, which matches how the work reads back:
// coming back to the styling after a week away was a fresh sitting, not a
// continuation. The connection between the two is real, but it belongs on a
// link between goals rather than in the grouping itself.
func Group(tasks []segment.Task, opt Options) []Goal {
	if len(tasks) == 0 {
		return nil
	}

	var goals []Goal
	cur := []segment.Task{tasks[0]}

	for i := 1; i < len(tasks); i++ {
		prev := tasks[i-1]
		next := tasks[i]

		if pause(prev, next) >= opt.Break {
			goals = append(goals, Goal{Tasks: cur})
			cur = nil
		}
		cur = append(cur, next)
	}
	return append(goals, Goal{Tasks: cur})
}

// pause is the time between the end of one task and the start of the next.
func pause(prev, next segment.Task) time.Duration {
	if len(prev.Turns) == 0 || len(next.Turns) == 0 {
		return 0
	}
	end := prev.Turns[len(prev.Turns)-1].At
	start := next.Turns[0].At
	if end.IsZero() || start.IsZero() {
		return 0
	}
	return start.Sub(end)
}

// Turns is every turn in the goal, in order, which is what the summary and the
// label are built from.
func (g Goal) Turns() []agent.Turn {
	var out []agent.Turn
	for _, t := range g.Tasks {
		out = append(out, t.Turns...)
	}
	return out
}
