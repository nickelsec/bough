package graph

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/repo"
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

// A session about one project regularly commits in another, a tool and its
// website worked on together being the usual case. Those commits are real but
// they are not this project's, and ten of one project's forty seven commit
// calls turned out to be a sibling repository's.
func TestCommitsMadeElsewhereAreNotThisProjects(t *testing.T) {
	for _, c := range []struct {
		name    string
		dir     string
		project string
		want    bool
	}{
		{"no cd is the project itself", "", "d:/boughs", true},
		{"the same place", "d:/boughs", "d:/boughs", true},
		{"a shell spelling of the same drive", "/d/boughs", "d:/boughs", true},
		{"windows separators", `d:\boughs`, "d:/boughs", true},
		{"a trailing separator", "d:/boughs/", "d:/boughs", true},
		{"a sibling repository", "/d/bough-site", "d:/boughs", false},
		{"a name that merely ends the same", "d:/my-boughs", "d:/boughs", false},
		{"somewhere else entirely", "d:/other", "d:/project", false},
	} {
		if got := here(c.dir, c.project); got != c.want {
			t.Errorf("%s: here(%q, %q) = %v, want %v", c.name, c.dir, c.project, got, c.want)
		}
	}
}

// A commit made after moving into a subdirectory still belongs to the project.
//
// `cd internal && git commit` runs in this repository. The recorded directory
// is the bare "internal", which never equalled an absolute project path, so
// the commit was dropped from the diagram with nothing said. Only an absolute
// path can name somewhere else, because only an absolute path says where it
// starts from.
func TestCommitInASubdirectoryIsKept(t *testing.T) {
	const proj = "/home/me/proj"
	for _, c := range []struct {
		dir  string
		want bool
		why  string
	}{
		{"", true, "a command that does not move runs where the session is"},
		{proj, true, "the project itself"},
		{"internal", true, "cd into a subdirectory is still this repository"},
		{"./internal", true, "the same, written with a leading dot"},
		{"internal/agent/codex", true, "deeper down is still inside"},
		{"/home/me/other", false, "an absolute path somewhere else"},
		{"../other", false, "relative, but it climbs out of the project"},
		{"..", false, "the parent directory is not this project"},
	} {
		if got := here(c.dir, proj); got != c.want {
			t.Errorf("here(%q) = %v, want %v: %s", c.dir, got, c.want, c.why)
		}
	}
}

// The two spellings of a Windows drive are one place, and a relative path is
// still relative whichever way the project is written.
func TestCommitDirAcrossDriveSpellings(t *testing.T) {
	for _, c := range []struct {
		dir, project string
		want         bool
	}{
		{"d:/work/site", "d:/work/site", true},
		{"/d/work/site", "d:/work/site", true},
		{"d:/work/other", "d:/work/site", false},
		{"internal", "d:/work/site", true},
	} {
		if got := here(c.dir, c.project); got != c.want {
			t.Errorf("here(%q, %q) = %v, want %v", c.dir, c.project, got, c.want)
		}
	}
}

// Matching a commit to the repository must not depend on the order the
// sessions happened to be walked in.
//
// Each repository commit goes to one agent commit, and it used to go to
// whichever reached it first. Two commits inside the same window meant the
// earlier-visited one took it, closer or not, and sessions are grouped by file
// rather than by time so that order is not even the order work happened in.
func TestCommitMatchingDoesNotDependOnOrder(t *testing.T) {
	when := func(s string) time.Time {
		v, err := time.Parse(time.RFC3339, s)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	have := []repo.Commit{
		{SHA: "aaa", When: when("2026-08-22T10:00:00Z")},
		{SHA: "bbb", When: when("2026-08-22T10:00:10Z")},
	}

	// A@10:00:05 is five seconds from either. B@10:00:00 is exactly on aaa.
	//
	// Settling in walk order gives aaa to A, because it is visited first and
	// aaa is as close as bbb, and B is left with bbb ten seconds away. Settling
	// the closest pair first gives aaa to B and bbb to A, which is the reading
	// that matches what happened.
	a := &agent.Commit{At: when("2026-08-22T10:00:05Z")}
	b := &agent.Commit{At: when("2026-08-22T10:00:00Z")}

	// Walked one way, then the other. The answer has to be the same.
	for _, order := range [][]*agent.Commit{{a, b}, {b, a}} {
		a.SHA, b.SHA = "", ""
		h := append([]repo.Commit(nil), have...)

		if missed := pair(order, h, matchWindow); len(missed) != 0 {
			t.Errorf("%d commits went unmatched, want 0", len(missed))
		}
		if b.SHA != "aaa" {
			t.Errorf("the commit sitting on aaa got %q, want aaa", b.SHA)
		}
		if a.SHA != "bbb" {
			t.Errorf("the commit five seconds from either got %q, want bbb", a.SHA)
		}
	}
}

// A repository commit is handed out once, and a commit with nothing near it
// comes back as unmatched rather than borrowing someone else's hash.
func TestPairHandsOutEachCommitOnce(t *testing.T) {
	when, err := time.Parse(time.RFC3339, "2026-08-22T10:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	have := []repo.Commit{{SHA: "aaa", When: when}}

	first := &agent.Commit{At: when.Add(time.Second)}
	second := &agent.Commit{At: when.Add(2 * time.Second)}
	far := &agent.Commit{At: when.Add(time.Hour)}
	zero := &agent.Commit{}

	missed := pair([]*agent.Commit{first, second, far, zero}, have, matchWindow)

	if first.SHA != "aaa" {
		t.Errorf("closest got %q, want aaa", first.SHA)
	}
	if second.SHA != "" {
		t.Errorf("second got %q, want nothing: aaa is spoken for", second.SHA)
	}
	if len(missed) != 3 {
		t.Errorf("%d unmatched, want 3", len(missed))
	}
}

// Build leaves the sessions it was given exactly as it found them.
//
// Everything inside writes into the turns: onlyHere drops commits made in
// another repository, and matching against git rewrites the hashes. Those
// edits used to land in the caller's own slices, so a second Build on one set
// of sessions saw the first one's leftovers and answered differently, and
// nothing else could reuse them afterwards.
func TestBuildDoesNotChangeTheSessionsItIsGiven(t *testing.T) {
	when := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	sessions := func() []agent.Session {
		return []agent.Session{{
			ID: "s1",
			Turns: []agent.Turn{{
				At: when, Text: "do it",
				Tools: map[string]int{}, Files: map[string]int{},
				Edits: map[string]int{}, Lines: map[string]int{},
				Committed: []agent.Commit{
					// One made somewhere else, which onlyHere drops.
					{SHA: "aaaa111", At: when, Dir: "/elsewhere"},
					{SHA: "bbbb222", At: when},
				},
			}},
		}}
	}

	given := sessions()
	opt := Options{Now: func() time.Time { return when }}

	first := Build(agent.Project{Name: "p", Path: "/p"}, given, opt)

	if got := len(given[0].Turns[0].Committed); got != 2 {
		t.Errorf("the caller's commits went from 2 to %d", got)
	}

	// And the same input twice gives the same answer, which is only true if
	// the first run left nothing behind.
	second := Build(agent.Project{Name: "p", Path: "/p"}, given, opt)
	if len(first.Goals) != len(second.Goals) {
		t.Errorf("two builds of one input: %d goals then %d", len(first.Goals), len(second.Goals))
	}
	if a, b := first.Totals.Commits, second.Totals.Commits; len(a) != len(b) {
		t.Errorf("two builds of one input: %d commits then %d", len(a), len(b))
	}
}

// A hash says whether the repository confirmed it.
//
// A commit carried the transcript's claim and one the repository had just
// verified in exactly the same shape, so a reader could not tell a hash worth
// looking up from one that may have been rebased away. The per project line
// already says whether the repository was read at all; this says it per commit,
// which is what a consumer of the JSON needs.
func TestACommitSaysWhetherItsHashWasConfirmed(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	made := agent.Turn{
		At: at, Text: "ship it",
		Tools: map[string]int{}, Files: map[string]int{}, Edits: map[string]int{},
		Lines:     map[string]int{},
		Committed: []agent.Commit{{Kind: "committed", SHA: "abc1234", At: at}},
	}
	in := []agent.Session{{ID: "s", Turns: []agent.Turn{made}}}
	p := agent.Project{Name: "app", Path: "/w/app"}

	// Nothing read, so the hash is the transcript's own claim.
	unread := Build(p, in, Options{})
	if c := firstCommit(t, unread); c.Confirmed {
		t.Error("a hash nothing checked is reported as confirmed")
	}

	// Read, and the repository has it.
	known := repo.History{
		Read:    true,
		Commits: []repo.Commit{{SHA: "abc1234", Subject: "ship it", When: at}},
	}
	read := Build(p, in, Options{Repo: known})
	c := firstCommit(t, read)
	if !c.Confirmed {
		t.Error("a hash the repository still has is not reported as confirmed")
	}
	if c.SHA != "abc1234" {
		t.Errorf("SHA = %q, want %q", c.SHA, "abc1234")
	}
}

func firstCommit(t *testing.T, g Graph) Commit {
	t.Helper()
	for _, goal := range g.Goals {
		for _, task := range goal.Tasks {
			for _, turn := range task.Turns {
				if len(turn.Committed) > 0 {
					return turn.Committed[0]
				}
			}
		}
	}
	t.Fatal("the graph has no commits in it")
	return Commit{}
}

// A task's token figure is the sum of its prompts', so the two can be checked
// against each other. They are worked out in different places: the turn copies
// what the agent charged, the task asks metrics to add them up. Nothing made
// them agree until this test, and a reader shown both would have believed
// whichever they read first.
func TestTaskTokensAreTheSumOfTheirPrompts(t *testing.T) {
	p, s := sample()
	// Uneven figures, so a sum that dropped one or counted it twice cannot
	// still come out right by accident.
	s[0].Turns[0].Tokens = agent.Tokens{Input: 11, Output: 22, CacheRead: 3300, CacheWrite: 440}
	s[0].Turns[1].Tokens = agent.Tokens{Input: 7, Output: 90, CacheRead: 1200}
	s[0].Turns[2].Tokens = agent.Tokens{Output: 5, CacheRead: 60}
	// The fourth is left unpaid, which is how a prompt that got no reply
	// arrives, and it must not become a zero anyone can see.

	g := Build(p, s, Options{Now: fixedNow})

	var prompts, tasks int
	for _, goal := range g.Goals {
		for _, task := range goal.Tasks {
			tasks++
			var summed int
			for _, turn := range task.Turns {
				if turn.Tokens == nil {
					continue
				}
				prompts++
				summed += turn.Tokens.Total()
			}
			var charged int
			if task.Stats.Tokens != nil {
				charged = task.Stats.Tokens.Total()
			}
			if summed != charged {
				t.Errorf("task %q: prompts add to %d, task says %d", task.Label, summed, charged)
			}
		}
	}
	if tasks == 0 {
		t.Fatal("no tasks, so nothing was actually compared")
	}
	if prompts != 3 {
		t.Errorf("charged prompts = %d, want 3: the unpaid one should carry no figure", prompts)
	}
}
