package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every project says which agent it came from.
//
// The listing used to name Codex and leave Claude Code blank, which read as
// though Claude were the absence of an agent rather than a choice of one. That
// was fair enough while there was only one agent to read, and it stopped being
// fair at the second.
func TestListNamesTheAgentForEveryProject(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "[Claude Code]") {
		t.Errorf("a Claude project did not say so:\n%s", out.String())
	}
}

// The columns line up, whatever is in them.
//
// The widths used to be fixed, and a Codex project is named after a directory
// that can run past them: one long row then pushed its own path and count out
// of line with every other row.
func TestListColumnsLineUp(t *testing.T) {
	root := twoProjects(t, "short", "a-considerably-longer-project-name")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected a line per project, got %d:\n%s", len(lines), out.String())
	}

	want := strings.Index(lines[0], "[")
	if want < 0 {
		t.Fatalf("no agent column in %q", lines[0])
	}
	for _, line := range lines[1:] {
		if at := strings.Index(line, "["); at != want {
			t.Errorf("agent column starts at %d here and %d on the first row:\n%s",
				at, want, out.String())
		}
	}
}

// twoProjects writes a history root holding two projects whose names are very
// different lengths, which is what makes the column question visible.
func twoProjects(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range names {
		dir := filepath.Join(root, "d--"+name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		line := `{"uuid":"1","type":"user","sessionId":"s-` + name + `","promptId":"p1",` +
			`"cwd":"/work/` + name + `","timestamp":"2026-08-01T09:00:00.000Z",` +
			`"message":{"role":"user","content":[{"type":"text","text":"a prompt long enough to count as a request rather than a nudge"}]}}`
		if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(line), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}
