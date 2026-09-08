package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
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
	projDir := filepath.Join(root, "--Users-alice-work-webapp--")
	if err := os.MkdirAll(projDir, 0o755); err != nil {
		t.Fatal(err)
	}

	transcript := filepath.Join(projDir, "session_1.jsonl")
	data := `{"type":"session","version":3,"id":"sess-123","timestamp":"2026-09-03T00:20:08.512Z","cwd":"/Users/alice/work/webapp"}
{"type":"message","id":"m1","timestamp":"2026-09-03T00:21:07.665Z","message":{"role":"user","content":[{"type":"text","text":"build the login feature"}]}}
{"type":"message","id":"m2","timestamp":"2026-09-03T00:21:10.784Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"tc1","name":"write","arguments":{"path":"login.go","content":"package main\nfunc Login() {}\n"}}],"model":"gpt-5.5","usage":{"input":50,"output":25,"cacheRead":100,"cacheWrite":0}}}
{"type":"message","id":"m3","timestamp":"2026-09-03T00:21:11.000Z","message":{"role":"toolResult","toolCallId":"tc1","toolName":"write","content":[{"type":"text","text":"ok"}],"isError":false}}
`
	if err := os.WriteFile(transcript, []byte(data), 0o644); err != nil {
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
	if p.Name != "webapp" {
		t.Errorf("expected name 'webapp', got %q", p.Name)
	}
	if p.Path != filepath.Clean("/Users/alice/work/webapp") {
		t.Errorf("expected path '/Users/alice/work/webapp', got %q", p.Path)
	}
	if p.Source != "pi" {
		t.Errorf("expected source 'pi', got %q", p.Source)
	}

	sessions, err := src.Sessions(p)
	if err != nil {
		t.Fatalf("Sessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "sess-123" {
		t.Errorf("expected session ID 'sess-123', got %q", sessions[0].ID)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(sessions[0].Turns))
	}

	turn := sessions[0].Turns[0]
	if turn.Text != "build the login feature" {
		t.Errorf("expected turn text 'build the login feature', got %q", turn.Text)
	}
	if turn.Files["login.go"] != 1 {
		t.Errorf("expected login.go touched once, got %d", turn.Files["login.go"])
	}
	if turn.Edits["login.go"] != 1 {
		t.Errorf("expected login.go edited once, got %d", turn.Edits["login.go"])
	}
	if turn.Tokens.Total() != 175 {
		t.Errorf("expected 175 tokens total, got %d", turn.Tokens.Total())
	}
}
