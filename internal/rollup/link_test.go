package rollup

import (
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/segment"
)

func goalOn(day int, edits map[string]int) Goal {
	t := agent.Turn{
		At:    time.Date(2026, 8, day, 10, 0, 0, 0, time.UTC),
		Files: map[string]int{},
		Edits: map[string]int{},
	}
	for f, n := range edits {
		t.Files[f] = n
		t.Edits[f] = n
	}
	return Goal{Tasks: []segment.Task{{Turns: []agent.Turn{t}}}}
}

func TestLinksFindResumedWork(t *testing.T) {
	goals := []Goal{
		goalOn(1, map[string]int{"parser.go": 5, "lexer.go": 2}),
		goalOn(3, map[string]int{"ui.css": 4}),
		goalOn(5, map[string]int{"parser.go": 3}),
	}
	links := Links(goals, DefaultLinkOptions())

	if len(links) != 1 {
		t.Fatalf("got %d links, want 1", len(links))
	}
	if links[0].From != 0 || links[0].To != 2 {
		t.Errorf("link joins %d and %d, want 0 and 2", links[0].From, links[0].To)
	}
	if links[0].Files[0] != "parser.go" {
		t.Errorf("files = %v, want parser.go", links[0].Files)
	}
	// The smaller side decides the weight, so a busy sitting cannot inflate a
	// link to one that barely touched the file.
	if links[0].Weight != 3 {
		t.Errorf("weight = %d, want 3", links[0].Weight)
	}
}

// Nothing shared means nothing drawn. Two of the five real sessions produced no
// links at all, and that has to stay quiet rather than inventing connections.
func TestLinksAreAbsentWhenNothingIsShared(t *testing.T) {
	goals := []Goal{
		goalOn(1, map[string]int{"a.go": 3}),
		goalOn(3, map[string]int{"b.go": 3}),
	}
	if got := Links(goals, DefaultLinkOptions()); len(got) != 0 {
		t.Errorf("got %d links, want none", len(got))
	}
}

// The agent rewrites its plan in nearly every sitting. Counting that would link
// every goal to every other goal and mean nothing.
func TestLinksIgnoreTheAgentsOwnFiles(t *testing.T) {
	plan := "/home/x/.claude/plans/session.md"
	goals := []Goal{
		goalOn(1, map[string]int{plan: 20}),
		goalOn(3, map[string]int{plan: 20}),
		goalOn(5, map[string]int{plan: 20}),
	}
	if got := Links(goals, DefaultLinkOptions()); len(got) != 0 {
		t.Errorf("got %d links from plan churn alone, want none", len(got))
	}
}

// A single passing edit on each side is a visit, not resumed work.
func TestLinksSkipIncidentalTouches(t *testing.T) {
	goals := []Goal{
		goalOn(1, map[string]int{"shared.go": 1}),
		goalOn(3, map[string]int{"shared.go": 1}),
	}
	if got := Links(goals, DefaultLinkOptions()); len(got) != 0 {
		t.Errorf("got %d links, want none for a single edit each side", len(got))
	}
}

func TestLinksAlwaysPointForwardInTime(t *testing.T) {
	goals := []Goal{
		goalOn(1, map[string]int{"x.go": 4}),
		goalOn(3, map[string]int{"x.go": 4}),
		goalOn(5, map[string]int{"x.go": 4}),
	}
	for _, l := range Links(goals, DefaultLinkOptions()) {
		if l.From >= l.To {
			t.Errorf("link runs backwards: %d to %d", l.From, l.To)
		}
	}
}

// The same history has to give the same picture every time.
func TestLinksAreDeterministic(t *testing.T) {
	goals := []Goal{
		goalOn(1, map[string]int{"a.go": 3, "b.go": 3, "c.go": 3}),
		goalOn(3, map[string]int{"a.go": 3, "b.go": 3, "c.go": 3}),
	}
	first := Links(goals, DefaultLinkOptions())
	for i := 0; i < 20; i++ {
		got := Links(goals, DefaultLinkOptions())
		if len(got) != len(first) {
			t.Fatal("link count changed between runs")
		}
		for j := range got {
			for k := range got[j].Files {
				if got[j].Files[k] != first[j].Files[k] {
					t.Fatalf("file order changed: %v then %v", first[j].Files, got[j].Files)
				}
			}
		}
	}
}

func TestLinksOnTooFewGoals(t *testing.T) {
	if Links(nil, DefaultLinkOptions()) != nil {
		t.Error("no goals should give no links")
	}
	if Links([]Goal{goalOn(1, map[string]int{"a.go": 5})}, DefaultLinkOptions()) != nil {
		t.Error("one goal cannot link to anything")
	}
}
