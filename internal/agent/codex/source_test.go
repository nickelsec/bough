package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
)

// Ensure Source satisfies agent.Source interface.
var _ agent.Source = Source{}

func TestDetectOnMissingRoot(t *testing.T) {
	projects, err := Source{Root: filepath.Join(t.TempDir(), "nonexistent")}.Detect()
	if err != nil {
		t.Fatalf("missing directory should not return an error, got: %v", err)
	}
	if len(projects) != 0 {
		t.Fatalf("expected 0 projects, got %d", len(projects))
	}
}

func TestDetectAndSessions(t *testing.T) {
	root := t.TempDir()
	dayDir := filepath.Join(root, "2026", "09", "08")
	if err := os.MkdirAll(dayDir, 0o755); err != nil {
		t.Fatal(err)
	}

	fixtureData, err := os.ReadFile("testdata/session.jsonl")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}

	rolloutPath := filepath.Join(dayDir, "rollout-2026-09-08T00-00-00-codex-sess-001.jsonl")
	if err := os.WriteFile(rolloutPath, fixtureData, 0o644); err != nil {
		t.Fatal(err)
	}

	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil {
		t.Fatalf("Detect failed: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}

	p := projects[0]
	if p.Name != "codex-app" {
		t.Errorf("expected name 'codex-app', got %q", p.Name)
	}
	if p.Path != filepath.Clean("/Users/alice/work/codex-app") {
		t.Errorf("expected path '/Users/alice/work/codex-app', got %q", p.Path)
	}
	if p.Source != "codex" {
		t.Errorf("expected source 'codex', got %q", p.Source)
	}

	sessions, err := src.Sessions(p)
	if err != nil {
		t.Fatalf("Sessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "codex-sess-001" {
		t.Errorf("expected session ID 'codex-sess-001', got %q", sessions[0].ID)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(sessions[0].Turns))
	}

	turn := sessions[0].Turns[0]
	if turn.Text != "implement user repository and commit" {
		t.Errorf("expected turn text 'implement user repository and commit', got %q", turn.Text)
	}
	expectedFile := shell.NormalisePath("/Users/alice/work/codex-app/repo.go")
	if turn.Files[expectedFile] != 1 {
		t.Errorf("expected repo.go touched once, got %d", turn.Files[expectedFile])
	}
	if turn.Edits[expectedFile] != 1 {
		t.Errorf("expected repo.go edited once, got %d", turn.Edits[expectedFile])
	}
	if len(turn.Committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(turn.Committed))
	}
	if turn.Committed[0].SHA != "1a2b3c4" {
		t.Errorf("expected SHA '1a2b3c4', got %q", turn.Committed[0].SHA)
	}
	if len(turn.Delegated) != 1 {
		t.Fatalf("expected 1 delegation, got %d", len(turn.Delegated))
	}
	if turn.Delegated[0].Kind != "db-tester" {
		t.Errorf("expected delegation kind 'db-tester', got %q", turn.Delegated[0].Kind)
	}
}

// Resuming a Codex session writes a new rollout file that replays the earlier
// items under the ids they already had. The repeats therefore sit across files
// rather than within one, and reading each file on its own never sees the
// first copy: two prompts came back as three across two sessions.
func TestResumedSessionIsOneSessionWithoutRepeats(t *testing.T) {
	dir := t.TempDir()
	day := filepath.Join(dir, "sessions", "2026", "09", "08")
	if err := os.MkdirAll(day, 0o755); err != nil {
		t.Fatal(err)
	}

	first := `{"timestamp":"2026-09-08T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/w/app"}}
{"timestamp":"2026-09-08T00:00:02Z","type":"response_item","payload":{"type":"message","id":"item-1","role":"user","content":[{"type":"input_text","text":"add the parser"}]}}
`
	// The resume replays item-1 verbatim before carrying on.
	second := `{"timestamp":"2026-09-08T01:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/w/app"}}
{"timestamp":"2026-09-08T00:00:02Z","type":"response_item","payload":{"type":"message","id":"item-1","role":"user","content":[{"type":"input_text","text":"add the parser"}]}}
{"timestamp":"2026-09-08T01:00:05Z","type":"response_item","payload":{"type":"message","id":"item-2","role":"user","content":[{"type":"input_text","text":"now the tests"}]}}
`
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(day, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("rollout-a.jsonl", first)
	write("rollout-b.jsonl", second)

	src := Source{Root: dir}
	projects, err := src.Detect()
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}

	sessions, err := src.Sessions(projects[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("got %d sessions, want 1: a resume is the same session", len(sessions))
	}

	var said []string
	for _, turn := range sessions[0].Turns {
		said = append(said, turn.Text)
	}
	if len(said) != 2 {
		t.Fatalf("got %d prompts %q, want 2: the replayed item was counted twice", len(said), said)
	}
	if said[0] != "add the parser" || said[1] != "now the tests" {
		t.Errorf("prompts = %q, want them in the order they were typed", said)
	}
}
