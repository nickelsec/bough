package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// The seam only holds if this actually satisfies the interface.
var _ agent.Source = Source{}

func TestDetectIgnoresNonTranscripts(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "d--example")
	if err := os.MkdirAll(filepath.Join(proj, "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(proj, "tool-results"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Files that live beside transcripts but are not sessions.
	write(t, filepath.Join(proj, "subagents", "agent-abc.meta.json"), `{"id":"abc"}`)
	write(t, filepath.Join(proj, "tool-results", "out.txt"), "some output")
	write(t, filepath.Join(proj, "notes.md"), "# notes")

	write(t, filepath.Join(proj, "1111.jsonl"),
		`{"uuid":"a","type":"user","promptId":"p1","cwd":"/work/example",`+
			`"message":{"role":"user","content":[{"type":"text","text":"do the thing"}]}}`)

	projects, err := Source{Root: root}.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	// The directory name is mangled and lossy, so the path comes off the record.
	if projects[0].Path != filepath.Clean("/work/example") {
		t.Errorf("path = %q, want the cwd from the record", projects[0].Path)
	}
	if projects[0].Name != "example" {
		t.Errorf("name = %q, want example", projects[0].Name)
	}
}

// A machine without Claude Code has no projects. That is not an error, so a
// caller can ask every agent it knows about without checking first.
func TestDetectOnMissingRoot(t *testing.T) {
	projects, err := Source{Root: filepath.Join(t.TempDir(), "nothing-here")}.Detect()
	if err != nil {
		t.Errorf("missing history should not be an error, got %v", err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want none", len(projects))
	}
}

// A directory holding only sidecar files is not a project.
func TestDetectSkipsProjectsWithoutTranscripts(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "d--empty")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "memory.md"), "notes")

	projects, err := Source{Root: root}.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want none", len(projects))
	}
}

func TestSessionsReadsTitleAndTurns(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "d--example")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "sess.jsonl"),
		`{"uuid":"a","type":"user","sessionId":"s1","promptId":"p1","cwd":"/work/example","timestamp":"2026-08-01T10:00:00.000Z",`+
			`"message":{"role":"user","content":[{"type":"text","text":"first request"}]}}`+"\n"+
			`{"type":"ai-title","aiTitle":"Reworking the export path","sessionId":"s1"}`)

	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := src.Sessions(projects[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1", len(sessions))
	}
	// Claude Code names its own sessions, so there is no need to infer one.
	if sessions[0].Title != "Reworking the export path" {
		t.Errorf("title = %q, want the agent's own label", sessions[0].Title)
	}
	if sessions[0].ID != "s1" {
		t.Errorf("id = %q, want s1", sessions[0].ID)
	}
	if len(sessions[0].Turns) != 1 {
		t.Errorf("got %d turns, want 1", len(sessions[0].Turns))
	}
}

// One unreadable transcript must not cost the caller the readable ones.
func TestSessionsSurvivesABadFile(t *testing.T) {
	root := t.TempDir()
	proj := filepath.Join(root, "d--example")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(proj, "good.jsonl"),
		`{"uuid":"a","type":"user","promptId":"p1","cwd":"/work/example","timestamp":"2026-08-01T10:00:00.000Z",`+
			`"message":{"role":"user","content":[{"type":"text","text":"a real request here"}]}}`)
	write(t, filepath.Join(proj, "bad.jsonl"), "{ not json at all\nalso not json")

	src := Source{Root: root}
	projects, _ := src.Detect()
	sessions, _ := src.Sessions(projects[0])

	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want the readable one", len(sessions))
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// A transcript written before Claude Code recorded promptId holds a real
// conversation that bough cannot read. Drawing nothing and saying nothing
// looks like lost work, so the reason is reported.
func TestSessionsReportsTranscriptsTooOldToRead(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "d--old")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"uuid":"a","type":"user","cwd":"/work/old","message":{"role":"user","content":[{"type":"text","text":"add a flag"}]}}` + "\n" +
		`{"uuid":"b","type":"user","cwd":"/work/old","message":{"role":"user","content":[{"type":"text","text":"now the docs"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	sessions, err := Source{Root: dir}.Sessions(agent.Project{Ref: proj})
	if len(sessions) != 0 {
		t.Fatalf("got %d sessions, want none readable", len(sessions))
	}
	if err == nil {
		t.Fatal("an unreadable transcript was skipped without saying so")
	}
	if !strings.Contains(err.Error(), "promptId") || !strings.Contains(err.Error(), "s1.jsonl") {
		t.Errorf("error = %q, want it to name the file and the reason", err)
	}
}

// An empty transcript, or one holding only tool traffic, is ordinary. Warning
// about those would bury the real case in noise.
func TestSessionsStaysQuietAboutEmptyTranscripts(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "d--quiet")
	if err := os.MkdirAll(proj, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := `{"uuid":"a","type":"user","cwd":"/work/q","message":{"role":"user","content":[{"type":"tool_result","is_error":false}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(proj, "s1.jsonl"), []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := (Source{Root: dir}).Sessions(agent.Project{Ref: proj}); err != nil {
		t.Errorf("an ordinary empty transcript was reported as a problem: %v", err)
	}
}
