package rollup

import (
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/segment"
)

func task(startMin, endMin int) segment.Task {
	base := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	return segment.Task{Turns: []agent.Turn{
		{At: base.Add(time.Duration(startMin) * time.Minute)},
		{At: base.Add(time.Duration(endMin) * time.Minute)},
	}}
}

func TestGroupSplitsOnOvernightBreak(t *testing.T) {
	tasks := []segment.Task{
		task(0, 30),
		task(60, 90),     // same sitting
		task(900, 930),   // next morning
		task(960, 990),   // same sitting
		task(2400, 2430), // day after
	}
	goals := Group(tasks, DefaultOptions())

	if len(goals) != 3 {
		t.Fatalf("got %d goals, want 3 sittings", len(goals))
	}
	if len(goals[0].Tasks) != 2 || len(goals[1].Tasks) != 2 || len(goals[2].Tasks) != 1 {
		t.Errorf("sitting sizes = %d,%d,%d, want 2,2,1",
			len(goals[0].Tasks), len(goals[1].Tasks), len(goals[2].Tasks))
	}
}

// A meal or a meeting is not the end of a piece of work.
func TestGroupKeepsShortBreaksTogether(t *testing.T) {
	tasks := []segment.Task{task(0, 30), task(200, 230), task(400, 430)}
	if got := len(Group(tasks, DefaultOptions())); got != 1 {
		t.Errorf("got %d goals, want 1", got)
	}
}

// Every task has to land in exactly one goal, or the view misrepresents the work.
func TestGroupLosesNothing(t *testing.T) {
	var tasks []segment.Task
	for i := 0; i < 30; i++ {
		tasks = append(tasks, task(i*200, i*200+30))
	}
	goals := Group(tasks, DefaultOptions())

	seen := 0
	for _, g := range goals {
		if len(g.Tasks) == 0 {
			t.Error("empty goal")
		}
		seen += len(g.Tasks)
	}
	if seen != len(tasks) {
		t.Errorf("goals hold %d tasks, input had %d", seen, len(tasks))
	}
}

func TestGroupHandlesEmpty(t *testing.T) {
	if Group(nil, DefaultOptions()) != nil {
		t.Error("no tasks should give no goals")
	}
}

// History without timestamps still has to produce something rather than crash.
func TestGroupToleratesMissingTimestamps(t *testing.T) {
	tasks := []segment.Task{
		{Turns: []agent.Turn{{}}},
		{Turns: []agent.Turn{{}}},
	}
	goals := Group(tasks, DefaultOptions())
	if len(goals) != 1 {
		t.Errorf("got %d goals, want 1", len(goals))
	}
}

func TestGoalTurnsAreInOrder(t *testing.T) {
	goals := Group([]segment.Task{task(0, 30), task(60, 90)}, DefaultOptions())
	turns := goals[0].Turns()
	if len(turns) != 4 {
		t.Fatalf("got %d turns, want 4", len(turns))
	}
	for i := 1; i < len(turns); i++ {
		if turns[i].At.Before(turns[i-1].At) {
			t.Error("turns came back out of order")
		}
	}
}
