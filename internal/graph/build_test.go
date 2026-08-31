package graph

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

var fixedNow = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }

func turn(day, min int, text string, edits ...string) agent.Turn {
	t := agent.Turn{
		At:    time.Date(2026, 8, day, 9, 0, 0, 0, time.UTC).Add(time.Duration(min) * time.Minute),
		Text:  text,
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
	}
	for _, f := range edits {
		t.Files[f]++
		t.Edits[f]++
	}
	return t
}

const longRequest = "rework the export path so a document with an embedded font renders its first page"
const otherRequest = "the search index needs to rebuild itself whenever a document is deleted from disk"

func sample() (agent.Project, []agent.Session) {
	p := agent.Project{Name: "example", Path: "/work/example", Source: "claude-code"}
	s := []agent.Session{{
		ID:    "s1",
		Title: "Working on the exporter",
		Turns: []agent.Turn{
			turn(1, 0, longRequest, "export.go"),
			turn(1, 20, "keep going", "export.go"),
			turn(2, 0, otherRequest, "search.go"),
			turn(2, 30, "now handle the empty case too", "search.go", "export.go"),
		},
	}}
	return p, s
}

func TestBuildProducesTheHierarchy(t *testing.T) {
	p, sessions := sample()
	opt := DefaultOptions()
	opt.Now = fixedNow

	g := Build(p, sessions, opt)

	if g.Schema != SchemaVersion {
		t.Errorf("schema = %d, want %d", g.Schema, SchemaVersion)
	}
	if g.Project.Name != "example" || g.Project.Agent != "claude-code" {
		t.Errorf("project = %+v", g.Project)
	}
	// Two days of work, so two sittings.
	if len(g.Goals) != 2 {
		t.Fatalf("got %d goals, want 2", len(g.Goals))
	}
	if g.Goals[0].ID != "g1" || g.Goals[1].ID != "g2" {
		t.Errorf("ids = %q, %q", g.Goals[0].ID, g.Goals[1].ID)
	}
	if g.Goals[0].Title != "Working on the exporter" {
		t.Errorf("title = %q, want the session's own name", g.Goals[0].Title)
	}
	if g.Totals.Turns != 4 {
		t.Errorf("totals.turns = %d, want 4", g.Totals.Turns)
	}
	// Task ids have to say which goal they belong to.
	if got := g.Goals[0].Tasks[0].ID; got != "g1.t1" {
		t.Errorf("task id = %q, want g1.t1", got)
	}
}

// The same history has to produce byte-identical output, or nobody can diff two
// runs to see what a change actually did.
func TestBuildIsDeterministic(t *testing.T) {
	p, sessions := sample()
	opt := DefaultOptions()
	opt.Now = fixedNow

	first, err := json.Marshal(Build(p, sessions, opt))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		again, err := json.Marshal(Build(p, sessions, opt))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, again) {
			t.Fatalf("output changed between runs\nfirst: %s\nagain: %s", first, again)
		}
	}
}

// Nothing about drawing belongs in the core's output. If one of these ever
// appears, the renderer has started bending the shape of the data.
func TestGraphCarriesNoPresentation(t *testing.T) {
	p, sessions := sample()
	opt := DefaultOptions()
	opt.Now = fixedNow

	body, err := json.Marshal(Build(p, sessions, opt))
	if err != nil {
		t.Fatal(err)
	}
	for _, banned := range []string{
		`"x"`, `"y"`, `"color"`, `"colour"`, `"width"`, `"height"`,
		`"radius"`, `"size"`, `"collapsed"`, `"expanded"`, `"zoom"`, `"font"`,
	} {
		if bytes.Contains(body, []byte(banned)) {
			t.Errorf("graph contains %s, which is a rendering concern", banned)
		}
	}
}

// Every turn has to survive into the output. A view that quietly drops work is
// worse than no view.
func TestBuildLosesNoTurns(t *testing.T) {
	p, sessions := sample()
	opt := DefaultOptions()
	opt.Now = fixedNow

	g := Build(p, sessions, opt)

	counted := 0
	for _, goal := range g.Goals {
		for _, task := range goal.Tasks {
			counted += len(task.Turns)
		}
	}
	want := 0
	for _, s := range sessions {
		want += len(s.Turns)
	}
	if counted != want {
		t.Errorf("graph holds %d turns, history had %d", counted, want)
	}
}

// Prompts are the user's own writing and the reason to click into anything, so
// they are never trimmed on the way out.
func TestTurnTextIsNotTruncated(t *testing.T) {
	long := longRequest + " " + longRequest + " " + longRequest
	p := agent.Project{Name: "x", Source: "claude-code"}
	sessions := []agent.Session{{Turns: []agent.Turn{turn(1, 0, long)}}}

	opt := DefaultOptions()
	opt.Now = fixedNow
	g := Build(p, sessions, opt)

	if got := g.Goals[0].Tasks[0].Turns[0].Text; got != long {
		t.Errorf("prompt was altered on the way out:\n got %q\nwant %q", got, long)
	}
}

// A project with several sessions should read as one run of work, since
// sessions are how the agent stores things rather than how the work happened.
func TestBuildOrdersGoalsAcrossSessions(t *testing.T) {
	p := agent.Project{Name: "x", Source: "claude-code"}
	sessions := []agent.Session{
		{ID: "later", Turns: []agent.Turn{turn(9, 0, longRequest)}},
		{ID: "earlier", Turns: []agent.Turn{turn(2, 0, otherRequest)}},
	}
	opt := DefaultOptions()
	opt.Now = fixedNow

	g := Build(p, sessions, opt)

	if len(g.Goals) != 2 {
		t.Fatalf("got %d goals, want 2", len(g.Goals))
	}
	if !g.Goals[0].Stats.Start.Before(g.Goals[1].Stats.Start) {
		t.Error("goals came back out of time order")
	}
}

func TestBuildOnEmptyHistory(t *testing.T) {
	opt := DefaultOptions()
	opt.Now = fixedNow
	g := Build(agent.Project{Name: "empty", Source: "claude-code"}, nil, opt)

	if len(g.Goals) != 0 || g.Totals.Turns != 0 {
		t.Errorf("empty history should give an empty graph, got %+v", g.Totals)
	}
	if _, err := json.Marshal(g); err != nil {
		t.Errorf("an empty graph must still serialise: %v", err)
	}
}
