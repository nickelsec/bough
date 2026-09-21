package graph

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

func minute(min int) time.Time {
	return time.Date(2026, 9, 9, 12, min, 0, 0, time.UTC)
}

func spent(when int, text string, in, out int) agent.Turn {
	return agent.Turn{
		At:    minute(when),
		Text:  text,
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
		Lines: map[string]int{},

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

	opt.Now = func() time.Time { return minute(50) }

	g := Build(agent.Project{Name: "site", Path: "/site"}, []agent.Session{parent, child}, opt)

	if len(g.Goals) != 1 {
		t.Fatalf("got %d goals, want 1: delegated work became a sitting of its own", len(g.Goals))
	}
	if g.Totals.Tokens.Input != 140 {
		t.Errorf("input = %d, want 140", g.Totals.Tokens.Input)
	}
}

// A Codex hand-off takes its name from the sub-agent's own transcript.
//
// The Codex reader puts a sub-agent's task name in TaskName and leaves Text
// empty, because nobody typed anything. describe read Text, so it returned
// early on every Codex hand-off and the spawn stayed nameless even though the
// transcript named the work. The existing fold tests put the name in Text, so
// they went on passing.
func TestDelegationTakesItsNameFromTheSubAgentsTranscript(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	parent := session("p", "", turnAt(at, func(tn *agent.Turn) {
		tn.Text = "make the art"
		tn.Delegated = []agent.Delegation{{Kind: "worker"}}
	}))
	kid := session("k", "p", turnAt(at.Add(time.Minute), func(tn *agent.Turn) {
		tn.TaskName = "/root/pixel_art"
	}))

	g := Build(agent.Project{Name: "app", Path: "/w/app"}, []agent.Session{parent, kid}, Options{})

	var sb strings.Builder
	if err := WriteText(&sb, g, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "pixel_art") {
		t.Errorf("the hand-off does not name the task the sub-agent's transcript recorded:\n%s", sb.String())
	}
}

// A sub-agent turn that was never folded still says what it was.
//
// Folding only happens when the parent session is present. Otherwise the turn
// stays a prompt of its own, and its task name never reached the graph, so the
// page showed an empty popover, the reader an empty line, and search could not
// find it at all.
func TestAnUnfoldedSubAgentTurnShowsItsTaskName(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	orphan := session("k", "gone", turnAt(at, func(tn *agent.Turn) {
		tn.TaskName = "/root/pixel_art"
	}))

	g := Build(agent.Project{Name: "app", Path: "/w/app"}, []agent.Session{orphan}, Options{})

	if n := len(g.Goals); n == 0 {
		t.Fatal("the turn produced no work at all")
	}
	var sb strings.Builder
	if err := WriteText(&sb, g, true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sb.String(), "pixel_art") {
		t.Errorf("an unfolded sub-agent turn shows as a blank prompt:\n%s", sb.String())
	}
}

// Every hand-off line says something.
//
// A spawn that recorded no description, no name and no kind printed
// "handed off:" and stopped, which is the bare line handoff exists to prevent.
func TestAHandoffWithNothingRecordedStillSaysSomething(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	s := session("p", "", turnAt(at, func(tn *agent.Turn) {
		tn.Text = "do it"
		tn.Delegated = []agent.Delegation{{}}
	}))

	g := Build(agent.Project{Name: "app", Path: "/w/app"}, []agent.Session{s}, Options{})

	var sb strings.Builder
	if err := WriteText(&sb, g, true); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(sb.String(), "\n") {
		if strings.Contains(line, "handed off:") && strings.TrimSpace(line) == "handed off:" {
			t.Error("a hand-off printed a bare line with nothing after the colon")
		}
	}
	if !strings.Contains(sb.String(), UnnamedHandoff) {
		t.Errorf("an unrecorded hand-off does not say so:\n%s", sb.String())
	}
}

// Building does not change the sessions it was given, sub-agents included.
//
// clone copies the turns and each turn's commit list, on the reasoning that the
// counting maps are read and never written. absorb writes them: folding a
// sub-agent adds its counts into the parent turn's own maps, which still belong
// to the caller. So one build changed the caller's data and a second build
// counted the sub-agent's work again on top of it.
func TestBuildDoesNotChangeSessionsWhenItFoldsASubAgent(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	parent := session("p", "", turnAt(at, func(tn *agent.Turn) {
		tn.Text = "make the art"
		tn.Tools["Bash"] = 1
		tn.Delegated = []agent.Delegation{{Kind: "worker"}}
	}))
	kid := session("k", "p", turnAt(at.Add(time.Minute), func(tn *agent.Turn) {
		tn.TaskName = "/root/pixel_art"
		tn.Tools["Bash"] = 2
	}))
	in := []agent.Session{parent, kid}
	before := deepCopy(in)

	p := agent.Project{Name: "app", Path: "/w/app"}
	first := Build(p, in, Options{})

	if !reflect.DeepEqual(in, before) {
		t.Errorf("Build changed the sessions it was given:\n got %+v\nwant %+v", in, before)
	}

	// And so the same input builds the same graph twice.
	second := Build(p, in, Options{})
	if !reflect.DeepEqual(first.Totals, second.Totals) {
		t.Errorf("two builds of one input disagree:\n first %+v\nsecond %+v", first.Totals, second.Totals)
	}
}

// session builds a session the way a reader would hand one over.
func session(id, parent string, turns ...agent.Turn) agent.Session {
	return agent.Session{ID: id, ParentID: parent, Turns: turns}
}

// turnAt builds a turn with every counting map ready, since a reader always
// provides them and code under test is entitled to assume it.
func turnAt(at time.Time, set func(*agent.Turn)) agent.Turn {
	t := agent.Turn{
		At:    at,
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
		Lines: map[string]int{},
	}
	if set != nil {
		set(&t)
	}
	return t
}

// deepCopy takes a copy deep enough to notice any write Build makes.
func deepCopy(in []agent.Session) []agent.Session {
	out := make([]agent.Session, len(in))
	for i, s := range in {
		s.Turns = append([]agent.Turn(nil), s.Turns...)
		for j := range s.Turns {
			t := &s.Turns[j]
			t.Tools = copyCounts(t.Tools)
			t.Files = copyCounts(t.Files)
			t.Edits = copyCounts(t.Edits)
			t.Lines = copyCounts(t.Lines)
			t.Models = copySpend(t.Models)
			t.Committed = append([]agent.Commit(nil), t.Committed...)
			t.Delegated = append([]agent.Delegation(nil), t.Delegated...)
		}
		out[i] = s
	}
	return out
}

func copyCounts(m map[string]int) map[string]int {
	if m == nil {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
