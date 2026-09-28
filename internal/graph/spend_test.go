package graph

import (
	"strings"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// charged builds a turn whose spend is attributed to one model.
func charged(min int, text, model string, tk agent.Tokens) agent.Turn {
	t := agent.Turn{
		At:     time.Date(2026, 9, 9, 12, min, 0, 0, time.UTC),
		Text:   text,
		Tools:  map[string]int{},
		Files:  map[string]int{},
		Edits:  map[string]int{},
		Lines:  map[string]int{},
		Tokens: tk,
	}
	agent.Charge(&t.Models, model, tk)
	return t
}

// sum adds up a graph Tokens set.
func sum(dst *Tokens, src *Tokens) {
	if src == nil {
		return
	}
	dst.Input += src.Input
	dst.Output += src.Output
	dst.CacheRead += src.CacheRead
	dst.CacheWrite += src.CacheWrite
	dst.CacheWriteHour += src.CacheWriteHour
}

// Everything has to add up, at every level, or a bill built from it is wrong
// in a way nothing on screen would show.
//
// Three things are checked together because they are one property: prompts sum
// to their task, tasks sum to the project, and within each of those the
// per-model split sums to the four-way total. A figure that passes two of the
// three and fails the last still puts a wrong number in front of someone.
func TestSpendReconcilesAtEveryLevel(t *testing.T) {
	one := agent.Tokens{Input: 10, Output: 20, CacheRead: 3000, CacheWrite: 40, CacheWriteHour: 30}
	two := agent.Tokens{Input: 1, Output: 2, CacheRead: 300, CacheWrite: 4, CacheWriteHour: 4}
	three := agent.Tokens{Input: 5, Output: 7, CacheRead: 900, CacheWrite: 11}

	sess := agent.Session{ID: "s", Turns: []agent.Turn{
		charged(0, "first", "claude-opus-5", one),
		charged(3, "second", "claude-opus-5", two),
		// A different model, so the per-model split has something to split.
		charged(400, "much later", "gpt-6-astra", three),
	}}

	opt := DefaultOptions()
	opt.Now = func() time.Time { return time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC) }
	g := Build(agent.Project{Name: "p", Path: "/p"}, []agent.Session{sess}, opt)

	// Every prompt sums to its task.
	var fromTasks Tokens
	for _, goal := range g.Goals {
		for _, task := range goal.Tasks {
			var fromTurns Tokens
			for _, turn := range task.Turns {
				sum(&fromTurns, turn.Tokens)
			}
			if task.Stats.Tokens == nil {
				t.Errorf("task %s has prompts but no figure", task.ID)
				continue
			}
			if fromTurns != *task.Stats.Tokens {
				t.Errorf("task %s: prompts sum to %+v, task says %+v",
					task.ID, fromTurns, *task.Stats.Tokens)
			}
			sum(&fromTasks, task.Stats.Tokens)
			checkModels(t, task.ID, task.Stats)
		}
	}

	// Every task sums to the project.
	if g.Totals.Tokens == nil {
		t.Fatal("the project has no figure at all")
	}
	if fromTasks != *g.Totals.Tokens {
		t.Errorf("tasks sum to %+v, project says %+v", fromTasks, *g.Totals.Tokens)
	}
	checkModels(t, "totals", g.Totals)

	// And nothing was lost on the way in.
	want := Tokens{
		Input:          one.Input + two.Input + three.Input,
		Output:         one.Output + two.Output + three.Output,
		CacheRead:      one.CacheRead + two.CacheRead + three.CacheRead,
		CacheWrite:     one.CacheWrite + two.CacheWrite + three.CacheWrite,
		CacheWriteHour: one.CacheWriteHour + two.CacheWriteHour + three.CacheWriteHour,
	}
	if *g.Totals.Tokens != want {
		t.Errorf("project total %+v, want %+v", *g.Totals.Tokens, want)
	}
}

// checkModels asserts the per-model split sums to the four-way total.
func checkModels(t *testing.T, where string, s Stats) {
	t.Helper()
	if len(s.Models) == 0 {
		return
	}
	var got Tokens
	for _, m := range s.Models {
		sum(&got, &m)
	}
	if s.Tokens == nil {
		t.Errorf("%s: has models but no token figure", where)
		return
	}
	if got != *s.Tokens {
		t.Errorf("%s: models sum to %+v, total says %+v", where, got, *s.Tokens)
	}
}

// A sub-agent's models survive being folded into a turn that had none.
//
// The merge used to give up when the parent's map was nil, and a Claude turn
// only grows one once a reply names a model. So a prompt that handed all its
// work to a sub-agent lost the record of which model did it, silently, while
// the tokens themselves arrived intact. That made the two disagree, which is
// exactly the shape of error a bill cannot survive.
func TestASubAgentsModelsSurviveTheFold(t *testing.T) {
	spend := agent.Tokens{Input: 40, Output: 4, CacheRead: 500, CacheWrite: 8}

	// The parent turn names no model of its own, so its map is nil.
	parent := agent.Turn{
		At:    time.Date(2026, 9, 9, 12, 30, 0, 0, time.UTC),
		Text:  "build it",
		Tools: map[string]int{}, Files: map[string]int{},
		Edits: map[string]int{}, Lines: map[string]int{},
		Delegated: []agent.Delegation{{Kind: "art"}},
	}
	child := charged(31, "/root/art", "claude-opus-5", spend)

	got := fold([]agent.Session{
		{ID: "S", Turns: []agent.Turn{parent}},
		{ID: "C", ParentID: "S", Turns: []agent.Turn{child}},
	})

	if len(got) != 1 || len(got[0].Turns) != 1 {
		t.Fatalf("fold produced %d sessions", len(got))
	}
	folded := got[0].Turns[0]
	if folded.Tokens != spend {
		t.Errorf("tokens = %+v, want %+v", folded.Tokens, spend)
	}
	if folded.Models["claude-opus-5"] != spend {
		t.Errorf("models = %v, want the sub-agent's model charged %+v", folded.Models, spend)
	}
}

// Work charged to nobody is still work, and must not vanish.
//
// A history recorded before models were named, or one whose records carry only
// the harness placeholder, has tokens and no model to hang them on. The token
// figure has to survive that; only the bill is refused.
func TestSpendWithoutAModelIsStillCounted(t *testing.T) {
	tk := agent.Tokens{Input: 10, Output: 20, CacheRead: 300}
	turn := agent.Turn{
		At:    time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC),
		Text:  "do it",
		Tools: map[string]int{}, Files: map[string]int{},
		Edits: map[string]int{}, Lines: map[string]int{},
		Tokens: tk,
	}

	opt := DefaultOptions()
	opt.Now = func() time.Time { return time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC) }
	g := Build(agent.Project{Name: "p", Path: "/p"},
		[]agent.Session{{ID: "s", Turns: []agent.Turn{turn}}}, opt)

	if g.Totals.Tokens == nil || g.Totals.Tokens.Total() != tk.Total() {
		t.Errorf("totals = %+v, want %d tokens", g.Totals.Tokens, tk.Total())
	}
	if len(g.Totals.Models) != 0 {
		t.Errorf("models = %v, want none: nothing named a model", g.Totals.Models)
	}
}

// Work done entirely on a free model is priced, at nothing, and the figure is
// there to say so. Leaving it off, as happened while every rate in the table
// was above zero, made free work read as work that could not be priced.
func TestFreeWorkIsPricedAtNothing(t *testing.T) {
	tk := agent.Tokens{Input: 700, Output: 60, CacheRead: 100}
	turn := charged(0, "it should keep changing", "openrouter/nvidia/nemotron-3-ultra-550b-a55b:free", tk)

	opt := DefaultOptions()
	opt.Now = func() time.Time { return time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC) }
	g := Build(agent.Project{Name: "p", Path: "/p"},
		[]agent.Session{{ID: "s", Turns: []agent.Turn{turn}}}, opt)

	if g.Totals.Cost == nil || *g.Totals.Cost != 0 {
		t.Errorf("project cost = %v, want a figure of zero", g.Totals.Cost)
	}
	if c := g.Totals.Spend(); !c.Priced || len(c.Unpriced) != 0 {
		t.Errorf("spend = %+v, want priced with nothing left out", c)
	}
	task := g.Goals[0].Tasks[0].Stats
	if task.Cost == nil || *task.Cost != 0 {
		t.Errorf("task cost = %v, want a figure of zero", task.Cost)
	}
}

// Work on a model with no published rate says NA where the figure would go,
// and names the model, rather than leaving the cost out without a word.
func TestUnpricedWorkSaysNA(t *testing.T) {
	tk := agent.Tokens{Input: 100, Output: 10}
	turn := charged(0, "ask the unknown model", "acme/acme-large", tk)

	opt := DefaultOptions()
	opt.Now = func() time.Time { return time.Date(2026, 9, 9, 23, 0, 0, 0, time.UTC) }
	g := Build(agent.Project{Name: "p", Path: "/p"},
		[]agent.Session{{ID: "s", Turns: []agent.Turn{turn}}}, opt)
	if g.Totals.Cost != nil {
		t.Fatalf("an unknown model was priced at %v", *g.Totals.Cost)
	}

	var b strings.Builder
	if err := WriteText(&b, g, false); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, "NA at API rates: no published rate for acme/acme-large") {
		t.Errorf("project line does not say NA:\n%s", out)
	}
	if !strings.Contains(out, "10 written, NA") {
		t.Errorf("task line does not say NA:\n%s", out)
	}
}

// Durations read the way a person would say them, singular included.
func TestHoursSaysOneMinute(t *testing.T) {
	for m, want := range map[int]string{
		0: "a moment", 1: "1 minute", 2: "2 minutes", 59: "59 minutes",
		60: "an hour", 61: "an hour and 1 minute", 90: "an hour and 30 minutes", 150: "2 hours",
	} {
		if got := hours(m); got != want {
			t.Errorf("hours(%d) = %q, want %q", m, got, want)
		}
	}
}

// Two sessions open at once in the same project. Each sitting is measured on
// its own and was always right; the project's total ran their prompts end to
// end, jumped back from the long one's last prompt to the short one's first,
// and counted the jump as negative time. With a big enough overlap the total
// went below zero. It is the time spent across both, counted once, and the
// project ends when its last prompt did.
func TestOverlappingSessionsAreCountedOnce(t *testing.T) {
	at := func(h, m int) time.Time { return time.Date(2026, 9, 20, h, m, 0, 0, time.UTC) }
	var long, short []agent.Turn
	for m := 0; m <= 8*60; m += 10 {
		long = append(long, charged(0, "long", "m", agent.Tokens{Output: 1}))
		long[len(long)-1].At = at(10, 0).Add(time.Duration(m) * time.Minute)
	}
	for m := 0; m <= 60; m += 10 {
		short = append(short, charged(0, "short", "m", agent.Tokens{Output: 1}))
		short[len(short)-1].At = at(11, 0).Add(time.Duration(m) * time.Minute)
	}

	opt := DefaultOptions()
	opt.Now = func() time.Time { return at(23, 0) }
	g := Build(agent.Project{Name: "p", Path: "/p"},
		[]agent.Session{{ID: "a", Turns: long}, {ID: "b", Turns: short}}, opt)

	// 10:00 to 18:00 with a prompt at least every ten minutes: eight hours,
	// however many of the short session's prompts land in between.
	if g.Totals.ActiveMinutes != 480 {
		t.Errorf("project active = %d minutes, want 480", g.Totals.ActiveMinutes)
	}
	if !g.Totals.End.Equal(at(18, 0)) {
		t.Errorf("project ends at %v, want 18:00 when its last prompt was", g.Totals.End)
	}
	if g.Totals.SpanMinutes != 480 {
		t.Errorf("project span = %d minutes, want 480", g.Totals.SpanMinutes)
	}
	// The sittings are measured on their own and stay as they were.
	for _, goal := range g.Goals {
		if m := goal.Stats.ActiveMinutes; m != 480 && m != 60 {
			t.Errorf("a sitting measured %d minutes", m)
		}
	}
}
