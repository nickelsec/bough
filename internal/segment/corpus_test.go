package segment

import (
	"testing"
	"time"

	"github.com/nickelsec/boughs/internal/agent"
)

// Segmentation is fitted to one person's history, so these guard the shape of
// the output rather than exact counts. A change that shatters sessions into
// single prompts, or merges a week into one task, fails here.
func TestSplitStaysInSensibleRange(t *testing.T) {
	// A session shaped like real work: several sittings over a few days, with
	// short follow-ups inside each.
	var turns []agent.Turn
	minute := 0
	for day := 0; day < 4; day++ {
		for sitting := 0; sitting < 3; sitting++ {
			turns = append(turns, at(minute,
				"rework the "+[]string{"checkout", "export", "search"}[sitting]+
					" path so it handles the empty case without falling over"))
			minute += 5
			for follow := 0; follow < 6; follow++ {
				turns = append(turns, at(minute, "go for it"))
				minute += 5
			}
			minute += 200 // break between sittings
		}
		minute += 600 // overnight
	}

	tasks := Split(turns, DefaultOptions())
	if len(tasks) < 8 || len(tasks) > 20 {
		t.Errorf("got %d tasks from %d turns, expected roughly one per sitting",
			len(tasks), len(turns))
	}

	// Every turn must land in exactly one task, or the view lies about the work.
	total := 0
	for _, tk := range tasks {
		if len(tk.Turns) == 0 {
			t.Error("empty task")
		}
		total += len(tk.Turns)
	}
	if total != len(turns) {
		t.Errorf("tasks hold %d turns, session had %d", total, len(turns))
	}
}

// Tasks must stay in order and never overlap in time.
func TestSplitPreservesOrder(t *testing.T) {
	var turns []agent.Turn
	for i := 0; i < 40; i++ {
		turns = append(turns, at(i*30, "adjust the layout so the panel sits beside the preview instead of under it"))
	}
	tasks := Split(turns, DefaultOptions())

	var last time.Time
	for i, tk := range tasks {
		first := tk.Turns[0].At
		if i > 0 && first.Before(last) {
			t.Errorf("task %d starts at %v, before the previous task ended at %v", i, first, last)
		}
		last = tk.Turns[len(tk.Turns)-1].At
	}
}
