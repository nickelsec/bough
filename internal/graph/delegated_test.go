package graph

import (
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

func minute(min int) time.Time {
	return time.Date(2026, 9, 9, 12, min, 0, 0, time.UTC)
}

func spent(when int, text string, in, out int) agent.Turn {
	return agent.Turn{
		At:     minute(when),
		Text:   text,
		Tools:  map[string]int{},
		Files:  map[string]int{},
		Edits:  map[string]int{},
		Lines:  map[string]int{},
		Models: map[string]int{},
		Tokens: agent.Tokens{Input: in, Output: out},
	}
}

// A sub-agent runs inside one turn of the session that spawned it. Left as a
// session of its own it became a second goal, which put a second date heading
// on a single afternoon.
func TestDelegatedWorkJoinsTheTurnThatAskedForIt(t *testing.T) {
	// The delegating turn is deliberately not the last one. A sub-agent that
	// ran at 12:31 belongs to the prompt at 12:30, and taking whichever turn
	// came last would put its work under "change the fonts", which happened
	// seven minutes after the sub-agent had finished.
	parent := agent.Session{ID: "S", Turns: []agent.Turn{
		spent(30, "make me a site", 100, 10),
		spent(38, "change the fonts", 50, 5),
		spent(44, "give it a cooler name", 20, 2),
	}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "pixel_art"}}

	child := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{
		spent(31, "/root/pixel_art", 40, 4),
	}}

	got := fold([]agent.Session{parent, child})
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1: delegated work is not a sitting of its own", len(got))
	}
	if len(got[0].Turns) != 3 {
		t.Fatalf("got %d turns, want 3: the sub-agent's brief is not a prompt", len(got[0].Turns))
	}

	// The work lands on the turn that delegated it, not the one after.
	if in := got[0].Turns[0].Tokens.Input; in != 140 {
		t.Errorf("first turn input = %d, want 140", in)
	}
	if in := got[0].Turns[1].Tokens.Input; in != 50 {
		t.Errorf("second turn input = %d, want 50: work landed on the wrong turn", in)
	}
	if in := got[0].Turns[2].Tokens.Input; in != 20 {
		t.Errorf("last turn input = %d, want 20: work landed on whichever turn came last", in)
	}
}

// Nothing may be lost in the move. The sub-agent's spend is part of what
// answering the prompt cost.
func TestFoldKeepsEveryToken(t *testing.T) {
	parent := agent.Session{ID: "S", Turns: []agent.Turn{spent(30, "build it", 100, 10)}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "art"}}
	child := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{spent(31, "/root/art", 40, 4)}}

	var in, out int
	for _, s := range fold([]agent.Session{parent, child}) {
		for _, tu := range s.Turns {
			in += tu.Tokens.Input
			out += tu.Tokens.Output
		}
	}
	if in != 140 || out != 14 {
		t.Errorf("in=%d out=%d, want 140 and 14", in, out)
	}
}

// The hand-off is already recorded from the spawn call, which names the task,
// while the sub-agent's own transcript names the path it runs at. They are one
// delegation and must not be drawn as two.
func TestOneDelegationIsNotShownTwice(t *testing.T) {
	parent := agent.Session{ID: "S", Turns: []agent.Turn{spent(30, "build it", 100, 10)}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "pixel_art"}}
	child := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{spent(31, "/root/pixel_art", 40, 4)}}

	got := fold([]agent.Session{parent, child})
	if n := len(got[0].Turns[0].Delegated); n != 1 {
		t.Fatalf("got %d hand-offs, want 1", n)
	}
	if kind := got[0].Turns[0].Delegated[0].Kind; kind != "pixel_art" {
		t.Errorf("kind = %q, want the name from the spawn call", kind)
	}
}

// A sub-agent's task name is not a description of the work. Goal labels prefer
// a brief over the reader's own prompt, so writing a path there named the whole
// sitting "/root/pixel_art".
func TestTaskNameIsNotTreatedAsABrief(t *testing.T) {
	parent := agent.Session{ID: "S", Turns: []agent.Turn{spent(30, "build it", 100, 10)}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "pixel_art"}}
	child := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{spent(31, "/root/pixel_art", 40, 4)}}

	got := fold([]agent.Session{parent, child})
	if d := got[0].Turns[0].Delegated[0].Description; d != "" {
		t.Errorf("description = %q, want empty: a task name is not a brief", d)
	}
}

// A sub-agent can spawn its own, and the deepest work still has to arrive.
func TestNestedDelegationIsKept(t *testing.T) {
	parent := agent.Session{ID: "S", Turns: []agent.Turn{spent(30, "build it", 100, 10)}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "art"}}
	mid := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{spent(31, "/root/art", 40, 4)}}
	mid.Turns[0].Delegated = []agent.Delegation{{Kind: "sprites"}}
	deep := agent.Session{ID: "D", ParentID: "C", Turns: []agent.Turn{spent(32, "/root/art/sprites", 7, 1)}}

	got := fold([]agent.Session{parent, mid, deep})
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1", len(got))
	}
	if in := got[0].Turns[0].Tokens.Input; in != 147 {
		t.Errorf("input = %d, want 147: work two levels down went missing", in)
	}
}

// A session whose parent is not among those read keeps its place. Showing work
// in the wrong spot is a smaller wrong than losing it.
func TestOrphanedDelegationIsKept(t *testing.T) {
	orphan := agent.Session{ID: "C", ParentID: "missing", Turns: []agent.Turn{spent(31, "/root/art", 40, 4)}}

	got := fold([]agent.Session{orphan})
	if len(got) != 1 {
		t.Fatalf("got %d sessions, want 1: an orphan must not be dropped", len(got))
	}
	if got[0].Turns[0].Tokens.Input != 40 {
		t.Error("the orphan's work was lost")
	}
}

// Sessions nobody delegated are left exactly as they were.
func TestOrdinarySessionsAreUntouched(t *testing.T) {
	a := agent.Session{ID: "A", Turns: []agent.Turn{spent(30, "one", 10, 1)}}
	b := agent.Session{ID: "B", Turns: []agent.Turn{spent(40, "two", 20, 2)}}

	got := fold([]agent.Session{a, b})
	if len(got) != 2 {
		t.Fatalf("got %d sessions, want 2", len(got))
	}
	if got[0].ID != "A" || got[1].ID != "B" {
		t.Error("order changed when nothing was delegated")
	}
}

// Build has to call fold. Everything above tests fold directly, so without
// this a graph could be built straight from unfolded sessions and every test
// here would still pass.
func TestBuildFoldsDelegatedWork(t *testing.T) {
	parent := agent.Session{ID: "S", Turns: []agent.Turn{spent(30, "make me a site", 100, 10)}}
	parent.Turns[0].Delegated = []agent.Delegation{{Kind: "pixel_art"}}
	child := agent.Session{ID: "C", ParentID: "S", Turns: []agent.Turn{spent(31, "/root/pixel_art", 40, 4)}}

	opt := DefaultOptions()
	opt.SkipRepo = true
	opt.Now = func() time.Time { return minute(50) }

	g := Build(agent.Project{Name: "site", Path: "/site"}, []agent.Session{parent, child}, opt)

	if len(g.Goals) != 1 {
		t.Fatalf("got %d goals, want 1: delegated work became a sitting of its own", len(g.Goals))
	}
	if g.Totals.Tokens.Input != 140 {
		t.Errorf("input = %d, want 140", g.Totals.Tokens.Input)
	}
}
