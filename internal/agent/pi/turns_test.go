package pi

import (
	"os"
	"testing"
)

func TestExtractTurnsFromFixture(t *testing.T) {
	f, err := os.Open("testdata/session.jsonl")
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	defer f.Close()

	recs, err := ReadRecords(f)
	if err != nil {
		t.Fatalf("ReadRecords failed: %v", err)
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}

	turn := turns[0]
	if turn.Text != "fix header styling and commit" {
		t.Errorf("unexpected turn text: %q", turn.Text)
	}

	// Tools
	if turn.Tools["read"] != 1 {
		t.Errorf("expected 1 read tool call, got %d", turn.Tools["read"])
	}
	if turn.Tools["write"] != 1 {
		t.Errorf("expected 1 write tool call, got %d", turn.Tools["write"])
	}
	if turn.Tools["bash"] != 1 {
		t.Errorf("expected 1 bash tool call, got %d", turn.Tools["bash"])
	}

	// Files and Edits
	expectedFile := "src/header.css"
	if turn.Files[expectedFile] != 2 { // 1 read + 1 write
		t.Errorf("expected 2 touches on %s, got %d", expectedFile, turn.Files[expectedFile])
	}
	if turn.Edits[expectedFile] != 1 {
		t.Errorf("expected 1 edit on %s, got %d", expectedFile, turn.Edits[expectedFile])
	}
	if turn.Lines[expectedFile] != 5 {
		t.Errorf("expected 5 lines written, got %d", turn.Lines[expectedFile])
	}

	// Tokens
	if turn.Tokens.Input != 370 { // 100 + 120 + 150
		t.Errorf("expected 370 input tokens, got %d", turn.Tokens.Input)
	}
	if turn.Tokens.Output != 65 { // 20 + 30 + 15
		t.Errorf("expected 65 output tokens, got %d", turn.Tokens.Output)
	}
	if turn.Tokens.CacheRead != 1800 { // 500 + 600 + 700
		t.Errorf("expected 1800 cacheRead tokens, got %d", turn.Tokens.CacheRead)
	}

	// Models
	if turn.Models["gpt-5.5"] != 65 {
		t.Errorf("expected 65 output tokens for gpt-5.5, got %d", turn.Models["gpt-5.5"])
	}

	// Commits
	if len(turn.Committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(turn.Committed))
	}
	if turn.Committed[0].Kind != "committed" {
		t.Errorf("expected kind 'committed', got %q", turn.Committed[0].Kind)
	}
}

func TestExtractTurnsIgnoresSyntheticCommands(t *testing.T) {
	recs := []*Record{
		{
			Type: "message",
			Message: &Message{
				Role: "user",
				Content: Content{
					Text: "/exit",
				},
			},
		},
		{
			Type: "message",
			Message: &Message{
				Role: "assistant",
				Content: Content{
					Text: "Goodbye!",
				},
			},
		},
	}

	turns := ExtractTurns(recs)
	if len(turns) != 0 {
		t.Fatalf("expected /exit prompt to be skipped, got %d turns", len(turns))
	}
}
