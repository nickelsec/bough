package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/pick"
)

// history writes a small transcript that looks like the real thing.
func history(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	addHistory(t, root, project)
	return root
}

func addHistory(t *testing.T, root, project string) {
	t.Helper()
	dir := filepath.Join(root, "d--"+project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"uuid":"1","type":"user","sessionId":"s1","promptId":"p1","cwd":"/work/` + project + `","timestamp":"2026-08-01T09:00:00.000Z",` +
			`"message":{"role":"user","content":[{"type":"text","text":"rework the export path so an embedded font renders on the first page"}]}}`,
		`{"uuid":"2","type":"assistant","timestamp":"2026-08-01T09:05:00.000Z","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Edit","input":{"file_path":"/work/` + project + `/export.go"}}]}}`,
		`{"type":"ai-title","aiTitle":"Fixing the exporter","sessionId":"s1"}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestListShowsProjects(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("listing did not mention the project:\n%s", out.String())
	}
}

func TestAgentFlagClaude(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--agent", "claude", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("listing did not mention the project:\n%s", out.String())
	}
}

func TestAgentFlagCodex(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "08", "01")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatal(err)
	}
	transcript := filepath.Join(dayDir, "rollout-s1.jsonl")
	data := `{"type":"session_meta","payload":{"id":"s1","cwd":"/work/my-codex-project"}}
{"type":"item_meta","payload":{"id":"item-1","turn_id":"turn-1"}}
{"type":"prompt","payload":{"text":"hello codex"}}
`
	if err := os.WriteFile(transcript, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	if err := run([]string{"--list", "--agent", "codex", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "my-codex-project") {
		t.Errorf("expected listing to include project 'my-codex-project', got:\n%s", out.String())
	}
}

func TestAgentFlagUnknown(t *testing.T) {
	var out, errs bytes.Buffer
	err := run([]string{"--list", "--agent", "unknown"}, testEnv(&out, &errs))
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestJSONOutputIsValidAndVersioned(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--json", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["schema"] == nil {
		t.Error("output carries no schema version, so a reader cannot tell what it is looking at")
	}
}

// Flags have to work on either side of the project name. The standard parser
// stops at the first non-flag, which would quietly ignore the flag and print
// the wrong thing.
func TestFlagsWorkAfterTheProjectName(t *testing.T) {
	root := history(t, "example")

	var before, after, errs bytes.Buffer
	if err := run([]string{"--json", "--root", root, "example"}, testEnv(&before, &errs)); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"example", "--json", "--root", root}, testEnv(&after, &errs)); err != nil {
		t.Fatal(err)
	}

	for _, b := range []*bytes.Buffer{&before, &after} {
		var parsed map[string]any
		if err := json.Unmarshal(b.Bytes(), &parsed); err != nil {
			t.Fatalf("expected JSON either way, got: %s", b.String())
		}
	}
}

func TestTextOutputIsReadable(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{"example", "prompt", "rework the export path"} {
		if !strings.Contains(body, want) {
			t.Errorf("text output is missing %q:\n%s", want, body)
		}
	}
}

// Someone with no history should get an explanation, not a stack trace or an
// empty screen.
//
// Pointed at an empty directory rather than at whatever this machine happens to
// have, so the test says the same thing everywhere. It used to skip on any
// machine with a history of its own, which is every machine bough is developed
// on, so it never ran where it mattered.
func TestMissingHistoryExplainsItself(t *testing.T) {
	var out, errs bytes.Buffer
	err := run([]string{"--root", filepath.Join(t.TempDir(), "nothing")}, testEnv(&out, &errs))

	if err == nil {
		t.Fatal("expected an error when there is no history")
	}
	// Every agent is named, from the registry rather than written out here, so
	// adding an agent does not mean editing this wording.
	for _, k := range agent.Agents {
		if !strings.Contains(err.Error(), k.Display) {
			t.Errorf("error should name %q, got: %v", k.Display, err)
		}
	}
}

// And the message names the directory that was searched, not the one that
// would have been searched by default.
//
// It used to read the location out of the registry whatever --root said, so
// somebody pointed at an empty directory was told to look in
// ~/.claude/projects, which nothing had opened.
func TestMissingHistoryNamesTheDirectoryItSearched(t *testing.T) {
	empty := filepath.Join(t.TempDir(), "nothing")
	var out, errs bytes.Buffer
	err := run([]string{"--root", empty}, testEnv(&out, &errs))

	if err == nil {
		t.Fatal("expected an error when there is no history")
	}
	if !strings.Contains(err.Error(), empty) {
		t.Errorf("error should name the directory it searched (%s), got: %v", empty, err)
	}
	if strings.Contains(err.Error(), "~/.claude/projects") {
		t.Errorf("error names a directory nothing searched, got: %v", err)
	}
}

// Every agent is read under a custom root, because --agent says which agents
// to read and --root says where to look.
//
// A custom root used to pick Claude Code outright, so --agent=all --root DIR
// ignored "all" and a Codex history under DIR was reported as missing.
func TestACustomRootStillReadsEveryAgent(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "2026", "09", "09")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"timestamp":"2026-09-09T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/w/rollup"}}`,
		`{"timestamp":"2026-09-09T00:01:00Z","type":"response_item","payload":{"type":"message","role":"user",` +
			`"content":[{"type":"input_text","text":"a prompt long enough to count as a request rather than a nudge"}]}}`,
	}
	name := filepath.Join(dir, "rollout-2026-09-09T00-00-00-aaaa.jsonl")
	if err := os.WriteFile(name, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"--list", "--root", root},
		{"--list", "--agent", "all", "--root", root},
	} {
		var out, errs bytes.Buffer
		if err := run(args, testEnv(&out, &errs)); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(out.String(), "rollup") {
			t.Errorf("%v did not find the Codex history:\n%s", args, out.String())
		}
	}
}

func TestUnknownProjectSuggestsList(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	err := run([]string{"nonsense", "--root", root}, testEnv(&out, &errs))
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !strings.Contains(err.Error(), "--list") {
		t.Errorf("error should point at --list, got: %v", err)
	}
}

func TestWritesToAFile(t *testing.T) {
	root := history(t, "example")
	dest := filepath.Join(t.TempDir(), "graph.json")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--json", "--root", root, "-o", dest}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Errorf("file does not hold valid JSON: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should go to the screen when writing to a file, got: %s", out.String())
	}
}

func TestCurrentProjectComesFirstAndIsMarked(t *testing.T) {
	const cwd = "/somewhere/here"
	projects := []agent.Project{
		{Name: "alpha", Path: "/somewhere/alpha"},
		{Name: "here", Path: cwd},
		{Name: "beta", Path: "/somewhere/beta"},
	}

	ordered, found := currentFirst(projects, cwd)
	if !found {
		t.Fatal("the working directory should have matched a project")
	}
	if ordered[0].Name != "here" {
		t.Errorf("first is %q, want the project we are standing in", ordered[0].Name)
	}
	// The rest keep their order, so the list does not reshuffle around the move.
	if ordered[1].Name != "alpha" || ordered[2].Name != "beta" {
		t.Errorf("the other projects were reordered: %q, %q", ordered[1].Name, ordered[2].Name)
	}
	if len(ordered) != len(projects) {
		t.Errorf("got %d projects, want %d", len(ordered), len(projects))
	}
}

func TestNoMarkerWhenNotInsideAProject(t *testing.T) {
	projects := []agent.Project{
		{Name: "alpha", Path: "/nowhere/alpha"},
		{Name: "beta", Path: "/nowhere/beta"},
	}
	ordered, found := currentFirst(projects, "/somewhere/else")

	if found {
		t.Error("no project should have matched")
	}
	if ordered[0].Name != "alpha" {
		t.Errorf("order changed when it should not have: %q first", ordered[0].Name)
	}
}

// Naming a project still goes straight there. The list is for when nothing was
// asked for.
func TestNamingAProjectSkipsTheList(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(errs.String(), "Which project?") {
		t.Error("naming a project should not ask which project")
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("expected the project's output, got:\n%s", out.String())
	}
}

// The page is the default, but only when somebody is watching. Anything
// redirected or piped has to keep behaving as it did before, or reading bough
// into a file starts opening windows.
func TestBrowserOnlyWhenSomebodyIsWatching(t *testing.T) {
	tests := []struct {
		name    string
		text    bool
		outFile string
		stdout  io.Writer
		want    bool
	}{
		{"piped somewhere", false, "", &bytes.Buffer{}, false},
		{"asked for text", true, "", &bytes.Buffer{}, false},
		{"writing to a file", false, "graph.txt", &bytes.Buffer{}, false},
		{"text wins over a file too", true, "graph.txt", &bytes.Buffer{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := useBrowser(tc.text, tc.outFile, tc.stdout); got != tc.want {
				t.Errorf("useBrowser() = %v, want %v", got, tc.want)
			}
		})
	}
}

// A pipe must produce the same text it always did, with no server and no wait.
func TestPipedOutputIsStillText(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	if !strings.Contains(body, "example") || !strings.Contains(body, "prompt") {
		t.Errorf("expected the text view, got:\n%s", body)
	}
	if strings.Contains(body, "<html") || strings.Contains(errs.String(), "http://") {
		t.Error("a browser was opened for output that is not going to a screen")
	}
}

// Both installs the readme documents go through the Go toolchain, and neither
// passes a version in. Reporting "dev" for those meant a bug report could not
// say which build it came from, and `go install ...@v0.3.4` said it too.
func TestVersionPrefersTheStampedValue(t *testing.T) {
	was := version
	defer func() { version = was }()

	version = "v1.2.3"
	if got := released(); got != "v1.2.3" {
		t.Errorf("released() = %q, want the stamped value", got)
	}
}

// Without a stamp it asks the toolchain, which knows the module version for
// anything installed by version and the revision for a build from a checkout.
// Either answers "which build is this"; "dev" does not.
//
// A test binary carries neither: the toolchain stamps it "(devel)" with no
// VCS settings, so "dev" is the right answer here and the real paths are
// covered by the shipped binary instead. What this pins is that the fallback
// runs at all and never returns an empty string.
func TestVersionFallsBackToBuildInfo(t *testing.T) {
	was := version
	defer func() { version = was }()

	version = ""
	got := released()
	if got == "" {
		t.Fatal("released() is empty")
	}
	t.Logf("released() in a test binary = %q", got)
}

// --version has to answer before anything reads the disk, so it works on a
// machine with no history at all.
func TestVersionFlagPrints(t *testing.T) {
	var out, errOut bytes.Buffer
	if err := run([]string{"--version"}, testEnv(&out, &errOut)); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Error("--version printed nothing")
	}
}

// Every agent bough can build is in the registry, and every agent in the
// registry can be built.
//
// The two are separate lists by necessity: the registry lives below the agent
// packages so the core can read it, and only main can import those packages to
// construct one. Nothing else makes them agree, and half-registering an agent
// is quiet in both directions. A source missing from the registry has no
// display name and no --agent word; a row with no source behind it accepts a
// flag that then finds nothing.
func TestEveryAgentIsBothRegisteredAndBuildable(t *testing.T) {
	built := buildable()

	for _, k := range agent.Agents {
		if _, ok := built[k.Source]; !ok {
			t.Errorf("%s is in the registry but nothing can build it", k.Source)
		}
		if k.Display == "" || k.Where == "" || len(k.Flag) == 0 {
			t.Errorf("%s is registered without a name, a place or a flag", k.Source)
		}
	}

	for source := range built {
		if _, ok := agent.Lookup(source); !ok {
			t.Errorf("%s can be built but is not in the registry", source)
		}
	}
}

// The interactive chooser writes to the streams run was handed, and backing
// out comes back as a value.
//
// It used to reach past them: choose discarded its writer, offer wrote the
// banner straight to os.Stderr, and cancelling called os.Exit(0), which skips
// every deferred close on the way out and makes this path impossible to drive
// from a test at all. That last part is why none of it was covered.
func TestChoosingWritesToTheGivenStreamsAndCancelsCleanly(t *testing.T) {
	projects := []agent.Project{
		{Name: "alpha", Path: "/somewhere/alpha"},
		{Name: "beta", Path: "/somewhere/beta"},
	}

	// Empty input: the numbered list reads a line, gets nothing, and treats
	// that as backing out.
	var out bytes.Buffer
	_, err := choose(projects, "", Env{In: strings.NewReader(""), Out: &out})

	if !errors.Is(err, pick.ErrCancelled) {
		t.Fatalf("err = %v, want a cancellation", err)
	}
	if out.Len() == 0 {
		t.Error("nothing was written to the writer it was given")
	}
	for _, want := range []string{"alpha", "beta", "Which project?"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("the offer did not mention %q:\n%s", want, out.String())
		}
	}
}

// Cancelling the chooser is a clean return from run, not a failure.
//
// This used to call a copy of the rule kept in this file, so it passed with the
// real check deleted from run. It goes through run now, which it can only do
// because run takes its input stream rather than reaching for the process's.
//
// Nothing to read is how backing out arrives here: the chooser falls back to
// the numbered list when its input is not a terminal, and a list that cannot
// read an answer has been cancelled.
func TestCancellingIsNotAFailure(t *testing.T) {
	root := twoProjects(t, "example", "other")
	var out, errs bytes.Buffer
	env := Env{In: strings.NewReader(""), Out: &out, Err: &errs}

	if err := run([]string{"--root", root, "--text"}, env); err != nil {
		t.Errorf("cancelling reached the caller as an error: %v", err)
	}
	// The list itself was printed, but nothing after it: backing out reads
	// no project.
	if strings.Contains(out.String(), "prompt across") {
		t.Errorf("cancelling still read a project:\n%s", out.String())
	}
}

// And choosing a project reaches the graph.
//
// The other half of the same gap: nothing drove the chooser through run, so
// neither branch of it was covered.
func TestChoosingFromTheListReadsThatProject(t *testing.T) {
	root := twoProjects(t, "example", "other")
	var out, errs bytes.Buffer
	env := Env{In: strings.NewReader("1\n"), Out: &out, Err: &errs}

	if err := run([]string{"--root", root, "--text"}, env); err != nil {
		t.Fatalf("choosing the first project failed: %v", err)
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("the chosen project was not read:\n%s", out.String())
	}
}

// An answer that is not one of the choices is an error, not a silent nothing.
func TestAChoiceThatIsNotOnTheListIsAnError(t *testing.T) {
	root := twoProjects(t, "example", "other")
	var out, errs bytes.Buffer
	env := Env{In: strings.NewReader("99\n"), Out: &out, Err: &errs}

	if err := run([]string{"--root", root, "--text"}, env); err == nil {
		t.Error("an impossible choice was accepted")
	}
}

// testEnv is run's world for a test that does not care about input: nothing to
// read, and no working directory, so no project is offered as the one you are
// standing in.
func testEnv(out, errs io.Writer) Env {
	return Env{In: strings.NewReader(""), Out: out, Err: errs}
}
