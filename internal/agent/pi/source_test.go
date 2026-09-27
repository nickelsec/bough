package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

var _ agent.Source = Source{}

// copyFixture puts a fixture file under root, in the folder Pi would use.
func copyFixture(t *testing.T, root, folder, name, as string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(root, folder)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, as)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// fixtureProject copies the three sessions Pi's own SessionManager wrote into
// a fresh root, in the per-project folder Pi would put them in, and returns
// the one project they make.
func fixtureProject(t *testing.T) (Source, agent.Project) {
	t.Helper()
	root := t.TempDir()
	for _, f := range []string{"main", "fork", "clone"} {
		copyFixture(t, root, "--C--work-app--", filepath.Join("sessions", f+".jsonl"), f+".jsonl")
	}
	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want the one all three sessions ran in", len(projects))
	}
	return src, projects[0]
}

func TestDetectOnMissingRoot(t *testing.T) {
	projects, err := Source{Root: filepath.Join(t.TempDir(), "nope")}.Detect()
	if err != nil || len(projects) != 0 {
		t.Fatalf("got %v, %v; a machine without Pi has no projects and no error", projects, err)
	}
}

// The folder name cannot be turned back into the path, so the path comes from
// the header.
func TestDetectReadsThePathFromTheHeader(t *testing.T) {
	_, p := fixtureProject(t)
	if p.Source != "pi" {
		t.Errorf("Source = %q", p.Source)
	}
	if !agent.SamePath(p.Path, `C:\work\app`) {
		t.Errorf("Path = %q, want the header's cwd", p.Path)
	}
	// "app" on every machine, not only the one that wrote the path.
	if p.Name != "app" {
		t.Errorf("Name = %q", p.Name)
	}
	if p.Bytes == 0 || p.LastWorked.IsZero() {
		t.Errorf("extent not read: %d bytes, last %v", p.Bytes, p.LastWorked)
	}
}

// A session directory set by hand holds every project's files side by side.
func TestDetectReadsAFlatSessionDirectory(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, "", "real.jsonl", "2026-09-24T18-34-41-984Z_x.jsonl")
	copyFixture(t, root, "", filepath.Join("sessions", "main.jsonl"), "2026-09-20T10-00-00-000Z_y.jsonl")
	projects, err := Source{Root: root}.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want one per working directory", len(projects))
	}
}

// Another agent's history under the same root is left alone. Claude Code's
// records carry a cwd too, so the header's type is what decides.
func TestDetectLeavesOtherAgentsFilesAlone(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("d--work-claude/s.jsonl", `{"type":"user","uuid":"u1","cwd":"/work/claude","message":{"role":"user","content":"hi"}}`+"\n")
	write("2026/09/08/rollout-x.jsonl", `{"type":"session_meta","payload":{"id":"x","cwd":"/work/codex"}}`+"\n")
	write("empty/nothing.jsonl", "")
	write("notes.txt", "not a session")

	projects, err := Source{Root: root}.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Fatalf("claimed %d projects that are not Pi's: %+v", len(projects), projects)
	}
}

// Pi reads its directories from the environment, and so does bough.
// PI_CODING_AGENT_SESSION_DIR wins over PI_CODING_AGENT_DIR, which wins over
// the default. See packages/coding-agent/src/config.ts.
func TestRootFollowsPisEnvironment(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "")
	t.Setenv("PI_CODING_AGENT_DIR", "")
	if got, _ := (Source{}).root(); got != filepath.Join(home, ".pi", "agent", "sessions") {
		t.Errorf("default root = %q", got)
	}

	t.Setenv("PI_CODING_AGENT_DIR", "~/elsewhere")
	if got, _ := (Source{}).root(); got != filepath.Join(home, "elsewhere", "sessions") {
		t.Errorf("with PI_CODING_AGENT_DIR, root = %q", got)
	}

	t.Setenv("PI_CODING_AGENT_SESSION_DIR", "/sessions/here")
	if got, _ := (Source{}).root(); got != "/sessions/here" {
		t.Errorf("with PI_CODING_AGENT_SESSION_DIR, root = %q", got)
	}

	if got, _ := (Source{Root: "/given"}).root(); got != "/given" {
		t.Errorf("an explicit root was overridden: %q", got)
	}
}

// A fork copies the conversation under the same ids and timestamps. Only what
// happened after the copy is the fork's own.
func TestAForkCountsOnlyItsOwnWork(t *testing.T) {
	src, p := fixtureProject(t)
	sessions, err := src.Sessions(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("got %d sessions, want main, fork and clone", len(sessions))
	}
	var prompts []string
	for _, s := range sessions {
		for _, turn := range s.Turns {
			prompts = append(prompts, turn.Text)
		}
	}
	// Seven prompts in all: five in the main session, one after the fork and
	// one after the clone. Counting the copies would make it seventeen.
	if len(prompts) != 7 {
		t.Errorf("got %d prompts, want 7: %q", len(prompts), prompts)
	}
	for _, s := range sessions {
		switch {
		case s.Title == "Readme and more":
			if len(s.Turns) != 5 {
				t.Errorf("main session has %d turns, want 5", len(s.Turns))
			}
		case len(s.Turns) == 1:
			got := s.Turns[0]
			if got.Text != "after the fork" && got.Text != "after the clone" {
				t.Errorf("a copy's only turn is %q", got.Text)
			}
			if got.Tokens.Input != 111 && got.Tokens.Input != 222 {
				t.Errorf("a copy was charged %d input tokens, want only its own reply's", got.Tokens.Input)
			}
		default:
			t.Errorf("unexpected session %q with %d turns", s.Title, len(s.Turns))
		}
	}
}

// With the original gone, the fork is the only record of what it copied, and
// the copy counts.
func TestAForkWhoseOriginalIsGoneKeepsTheCopy(t *testing.T) {
	root := t.TempDir()
	copyFixture(t, root, "--C--work-app--", filepath.Join("sessions", "fork.jsonl"), "fork.jsonl")
	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil || len(projects) != 1 {
		t.Fatalf("%v, %v", projects, err)
	}
	sessions, err := src.Sessions(projects[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || len(sessions[0].Turns) != 6 {
		t.Fatalf("got %d sessions; want the fork alone with all six of its prompts", len(sessions))
	}
}

// A chain of forks that loops back on itself must not hang the reader.
func TestAForkLoopEnds(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.jsonl")
	b := filepath.Join(dir, "b.jsonl")
	body := func(parent string) string {
		return `{"type":"session","version":3,"id":"s","timestamp":"2026-09-20T10:00:00Z","cwd":"/w","parentSession":` +
			quote(parent) + "}\n" +
			`{"type":"message","id":"e1","parentId":null,"timestamp":"2026-09-20T10:00:01Z","message":{"role":"user","content":"hi","timestamp":1}}` + "\n"
	}
	if err := os.WriteFile(a, []byte(body(b)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(body(a)), 0o644); err != nil {
		t.Fatal(err)
	}
	keys := ancestry(b, a)
	if !keys["e1|2026-09-20T10:00:01Z"] {
		t.Error("the parent's entries were not gathered")
	}
}

// quote writes a string as a JSON string literal.
func quote(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

// The name given with /name is the title, and a later rename wins.
func TestTitleIsTheLatestName(t *testing.T) {
	src, p := fixtureProject(t)
	sessions, err := src.Sessions(p)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, s := range sessions {
		if s.Title == "Readme and more" {
			found = true
		}
		if s.Title == "Readme work" {
			t.Error("the first name was kept after a rename")
		}
	}
	if !found {
		t.Error("the session's name was not read")
	}
}

// A file that cannot be read is reported without losing the rest.
func TestSessionsSurvivesAMissingFile(t *testing.T) {
	src, p := fixtureProject(t)
	p.Ref = strings.Replace(p.Ref, `[`, `["`+strings.ReplaceAll(filepath.Join(t.TempDir(), "gone.jsonl"), `\`, `\\`)+`",`, 1)
	sessions, err := src.Sessions(p)
	if err == nil {
		t.Error("a missing file went unreported")
	}
	if len(sessions) != 3 {
		t.Errorf("got %d sessions, want the three that could be read", len(sessions))
	}
}

func TestSessionsRejectsABadRef(t *testing.T) {
	if _, err := (Source{}).Sessions(agent.Project{Name: "x", Ref: "not json"}); err == nil {
		t.Error("a malformed ref was read as though it were fine")
	}
}

func TestLastElemSplitsOnEitherSeparator(t *testing.T) {
	for in, want := range map[string]string{
		`C:\work\app`:         "app",
		`C:\Users\someone\`:   "someone",
		"/home/u/app":         "app",
		"/home/u/app/":        "app",
		`D:\pi-test`:          "pi-test",
		"relative":            "relative",
		`\server\share\thing`: "thing",
	} {
		if got := lastElem(in); got != want {
			t.Errorf("lastElem(%q) = %q, want %q", in, got, want)
		}
	}
}
