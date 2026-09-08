package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// history writes a small transcript that looks like the real thing.
func history(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
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
	return root
}

func TestListShowsProjects(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("listing did not mention the project:\n%s", out.String())
	}
}

func TestAgentFlagClaude(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--agent", "claude", "--root", root}, &out, &errs); err != nil {
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
	if err := run([]string{"--list", "--agent", "codex", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "my-codex-project") {
		t.Errorf("expected listing to include project 'my-codex-project', got:\n%s", out.String())
	}
}

func TestAgentFlagUnknown(t *testing.T) {
	var out, errs bytes.Buffer
	err := run([]string{"--list", "--agent", "unknown"}, &out, &errs)
	if err == nil || !strings.Contains(err.Error(), "unknown agent") {
		t.Fatalf("expected unknown agent error, got %v", err)
	}
}

func TestJSONOutputIsValidAndVersioned(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--json", "--root", root}, &out, &errs); err != nil {
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
	if err := run([]string{"--json", "--root", root, "example"}, &before, &errs); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"example", "--json", "--root", root}, &after, &errs); err != nil {
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

	if err := run([]string{"example", "--root", root}, &out, &errs); err != nil {
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
func TestMissingHistoryExplainsItself(t *testing.T) {
	var out, errs bytes.Buffer
	err := run([]string{"--root", filepath.Join(t.TempDir(), "nothing")}, &out, &errs)

	if err == nil {
		t.Fatal("expected an error when there is no history")
	}
	if !strings.Contains(err.Error(), "no Claude Code history") {
		t.Errorf("error should say what was looked for, got: %v", err)
	}
}

func TestUnknownProjectSuggestsList(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	err := run([]string{"nonsense", "--root", root}, &out, &errs)
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

	if err := run([]string{"example", "--json", "--root", root, "-o", dest}, &out, &errs); err != nil {
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
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	projects := []agent.Project{
		{Name: "alpha", Path: "/somewhere/alpha"},
		{Name: "here", Path: cwd},
		{Name: "beta", Path: "/somewhere/beta"},
	}

	ordered, found := currentFirst(projects)
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
	ordered, found := currentFirst(projects)

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

	if err := run([]string{"example", "--root", root}, &out, &errs); err != nil {
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

	if err := run([]string{"example", "--root", root}, &out, &errs); err != nil {
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
	if err := run([]string{"--version"}, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got == "" {
		t.Error("--version printed nothing")
	}
}
