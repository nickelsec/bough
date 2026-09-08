package antigravity

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
	logsDir := filepath.Join(root, "conv-xyz-123", ".system_generated", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	transcript := filepath.Join(logsDir, "transcript.jsonl")
	data := `{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-09-08T10:00:00Z","content":"<USER_REQUEST>\nbuild the auth service\n</USER_REQUEST>"}
{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-09-08T10:00:05Z","tool_calls":[{"name":"write_to_file","args":{"TargetFile":"\"/Users/alice/projects/auth/main.go\"","CodeContent":"\"package main\nfunc main() {}\n\""}}]}
{"step_index":2,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-09-08T10:00:06Z","content":"File created successfully"}
{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-09-08T10:00:10Z","tool_calls":[{"name":"run_command","args":{"CommandLine":"\"git commit -m \\\"init auth\\\"\"","Cwd":"\"/Users/alice/projects/auth\""}}]}
{"step_index":4,"source":"MODEL","type":"GENERIC","status":"DONE","created_at":"2026-09-08T10:00:12Z","content":"[main abc1234] init auth\n 1 file changed\nThe command exited with code 0."}
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
	if p.Name != "auth" {
		t.Errorf("expected name 'auth', got %q", p.Name)
	}
	if p.Path != filepath.Clean("/Users/alice/projects/auth") {
		t.Errorf("expected path '/Users/alice/projects/auth', got %q", p.Path)
	}
	if p.Source != "antigravity" {
		t.Errorf("expected source 'antigravity', got %q", p.Source)
	}

	sessions, err := src.Sessions(p)
	if err != nil {
		t.Fatalf("Sessions failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "conv-xyz-123" {
		t.Errorf("expected session ID 'conv-xyz-123', got %q", sessions[0].ID)
	}
	if len(sessions[0].Turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(sessions[0].Turns))
	}

	turn := sessions[0].Turns[0]
	if turn.Text != "build the auth service" {
		t.Errorf("expected turn text 'build the auth service', got %q", turn.Text)
	}
	expectedFile := normalisePath("/Users/alice/projects/auth/main.go")
	if turn.Files[expectedFile] != 1 {
		t.Errorf("expected file touched once, got %d", turn.Files[expectedFile])
	}
	if turn.Edits[expectedFile] != 1 {
		t.Errorf("expected file edited once, got %d", turn.Edits[expectedFile])
	}
	if len(turn.Committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(turn.Committed))
	}
	if turn.Committed[0].SHA != "abc1234" {
		t.Errorf("expected SHA abc1234, got %q", turn.Committed[0].SHA)
	}
}
