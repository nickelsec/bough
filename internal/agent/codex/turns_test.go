package codex

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/agent/shell"
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
	if turn.Text != "implement user repository and commit" {
		t.Errorf("unexpected turn text: %q", turn.Text)
	}

	// Tools
	if turn.Tools["apply_patch"] != 1 {
		t.Errorf("expected 1 apply_patch, got %d", turn.Tools["apply_patch"])
	}
	if turn.Tools["exec_command"] != 1 {
		t.Errorf("expected 1 exec_command, got %d", turn.Tools["exec_command"])
	}
	if turn.Tools["spawn_agent"] != 1 {
		t.Errorf("expected 1 spawn_agent, got %d", turn.Tools["spawn_agent"])
	}

	// Files and Edits
	repoGo := shell.NormalisePath("/Users/alice/work/codex-app/repo.go")
	if turn.Files[repoGo] != 1 {
		t.Errorf("expected 1 touch on repo.go, got %d", turn.Files[repoGo])
	}
	if turn.Edits[repoGo] != 1 {
		t.Errorf("expected 1 edit on repo.go, got %d", turn.Edits[repoGo])
	}
	if turn.Lines[repoGo] != 3 {
		t.Errorf("expected 3 lines on repo.go, got %d", turn.Lines[repoGo])
	}

	// Commit
	if len(turn.Committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(turn.Committed))
	}
	if turn.Committed[0].Kind != "committed" {
		t.Errorf("expected kind 'committed', got %q", turn.Committed[0].Kind)
	}
	if turn.Committed[0].SHA != "1a2b3c4" {
		t.Errorf("expected SHA '1a2b3c4', got %q", turn.Committed[0].SHA)
	}

	// Delegated subagent
	if len(turn.Delegated) != 1 {
		t.Fatalf("expected 1 delegation, got %d", len(turn.Delegated))
	}
	if turn.Delegated[0].Kind != "db-tester" {
		t.Errorf("expected kind 'db-tester', got %q", turn.Delegated[0].Kind)
	}
	if turn.Delegated[0].Description != "test db connections" {
		t.Errorf("expected description 'test db connections', got %q", turn.Delegated[0].Description)
	}

	// Tokens & Models
	if turn.Tokens.Input != 200 {
		t.Errorf("expected 200 input tokens, got %d", turn.Tokens.Input)
	}
	if turn.Tokens.CacheRead != 1500 {
		t.Errorf("expected 1500 cached tokens, got %d", turn.Tokens.CacheRead)
	}
	if turn.Tokens.Output != 80 {
		t.Errorf("expected 80 output tokens, got %d", turn.Tokens.Output)
	}
	if turn.Models["gpt-5.5"] != 80 {
		t.Errorf("expected 80 tokens for gpt-5.5, got %d", turn.Models["gpt-5.5"])
	}
}

func TestExtractTurnsErrors(t *testing.T) {
	outRaw, err := json.Marshal("Process exited with code 2\nError: compilation failed")
	if err != nil {
		t.Fatal(err)
	}
	recs := []*Record{
		{
			Timestamp: "2026-09-08T12:00:00Z",
			Type:      "response_item",
			Payload: json.RawMessage(`{
				"type": "message",
				"role": "user",
				"content": [{"type": "input_text", "text": "build project"}]
			}`),
		},
		{
			Timestamp: "2026-09-08T12:00:01Z",
			Type:      "response_item",
			Payload: json.RawMessage(`{
				"type": "function_call",
				"name": "exec_command",
				"arguments": "{\"cmd\": \"make build\"}"
			}`),
		},
		{
			Timestamp: "2026-09-08T12:00:05Z",
			Type:      "response_item",
			Payload: json.RawMessage(`{
				"type": "function_call_output",
				"output": ` + string(outRaw) + `
			}`),
		},
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Errors != 1 {
		t.Errorf("expected 1 error, got %d", turns[0].Errors)
	}
}

func TestPromptClean(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"# AGENTS.md instructions for /repo\n<INSTRUCTIONS>foo</INSTRUCTIONS>", ""},
		{"<environment_context><cwd>/repo</cwd></environment_context>", ""},
		{"Reviewed Codex session id: 123", ""},
		{"<user_shell_command>\n<command>\ngit status\n</command>\n<result>ok</result>\n</user_shell_command>", "git status"},
		{"Please refactor auth.go", "Please refactor auth.go"},
	}

	for _, tc := range cases {
		got := cleanPrompt(tc.input)
		if got != tc.want {
			t.Errorf("cleanPrompt(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// A patch may carry several files, and the lines under each header belong to
// that file. Counting the whole patch and crediting the first file named it a
// rewrite and left the rest looking untouched, which is the wrong answer for a
// score that reads how much a file moved. Removals count too, as they do on
// the Claude side.
func TestPatchLinesLandOnTheirOwnFile(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Update File: /w/app/one.go\n" +
		"+added to one\n" +
		"+added to one again\n" +
		"-removed from one\n" +
		"*** Update File: /w/app/two.go\n" +
		"+added to two\n" +
		"*** End Patch\n"

	turn := agent.Turn{
		Tools: map[string]int{},
		Files: map[string]int{},
		Edits: map[string]int{},
		Lines: map[string]int{},
	}
	applyPatch(&turn, patch)

	one := shell.NormalisePath("/w/app/one.go")
	two := shell.NormalisePath("/w/app/two.go")

	if got := turn.Lines[one]; got != 3 {
		t.Errorf("%s changed %d lines, want 3 (two added, one removed)", one, got)
	}
	if got := turn.Lines[two]; got != 1 {
		t.Errorf("%s changed %d lines, want 1", two, got)
	}
	if turn.Edits[one] != 1 || turn.Edits[two] != 1 {
		t.Errorf("edits = %v, want one apiece", turn.Edits)
	}
}
