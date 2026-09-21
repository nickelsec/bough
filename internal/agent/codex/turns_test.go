package codex

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
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
	repoGo := agent.NormalisePath("/Users/alice/work/codex-app/repo.go")
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
	// The whole four-way split is attributed to the model, not just what it
	// wrote. Pricing needs all four, since they are charged at rates that
	// differ by a factor of fifty.
	got := turn.Models["gpt-5.5"]
	want := agent.Tokens{Input: 200, Output: 80, CacheRead: 1500}
	if got != want {
		t.Errorf("gpt-5.5 charged %+v, want %+v", got, want)
	}
	// And what the models were charged sums to what the turn was charged.
	if got.Total() != turn.Tokens.Total() {
		t.Errorf("models total %d, turn total %d", got.Total(), turn.Tokens.Total())
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

	one := agent.NormalisePath("/w/app/one.go")
	two := agent.NormalisePath("/w/app/two.go")

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

// Every added or removed line counts, whatever text it starts with.
//
// The counter skipped any line opening "+++" or "---", on the reading that
// those name files. They do in a unified diff, but Codex names files with
// "*** Update File:" and never writes those headers, so in this format they
// are ordinary changed lines: adding "++i;" is "+++i;", and removing a Lua or
// SQL comment "-- note" is "--- note".
func TestPatchCountsLinesThatStartWithPlusOrMinus(t *testing.T) {
	patch := "*** Begin Patch\n" +
		"*** Update File: /src/a.c\n" +
		"+++i;\n" +
		"---j;\n" +
		"+x;\n" +
		"*** End Patch"

	cur := agent.Turn{
		Files: map[string]int{}, Edits: map[string]int{},
		Lines: map[string]int{}, Tools: map[string]int{},
	}
	applyPatch(&cur, patch)

	if got := cur.Lines["/src/a.c"]; got != 3 {
		t.Errorf("Lines = %d, want 3; every one of the three lines changed something", got)
	}
}

// A commit whose output arrives after the next prompt still counts.
//
// Opening a turn used to clear every command still waiting for its result, so
// a commit made just before the reader typed again vanished. The turn each
// call belongs to is already recorded when the call is seen, so the output can
// settle against it whenever it turns up. Claude Code keeps its pending
// commands across turns, and the two readers have to agree about the same
// sequence of events or one agent's history is quietly worse than the other's.
func TestCommitCountsWhenItsOutputArrivesAfterTheNextPrompt(t *testing.T) {
	recs := records(t,
		prompt("commit it"),
		functionCall("c1", "exec_command", `{"cmd":"git commit -m x"}`),
		prompt("next thing"),
		functionOutput("c1", "Process exited with code 0\n[main abc1234] x"),
	)

	turns := ExtractTurns(recs)
	if len(turns) != 2 {
		t.Fatalf("expected 2 turns, got %d", len(turns))
	}
	if n := len(turns[0].Committed); n != 1 {
		t.Fatalf("the commit was credited to %d turns, want it on the turn that made it", n)
	}
	if sha := turns[0].Committed[0].SHA; sha != "abc1234" {
		t.Errorf("SHA = %q, want %q", sha, "abc1234")
	}
	if n := len(turns[1].Committed); n != 0 {
		t.Errorf("the later turn got %d commits, want 0", n)
	}
}

func records(t *testing.T, payloads ...map[string]any) []*Record {
	t.Helper()
	out := make([]*Record, 0, len(payloads))
	for i, p := range payloads {
		raw, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshalling record %d: %v", i, err)
		}
		out = append(out, &Record{Type: "response_item", Payload: raw})
	}
	return out
}

func prompt(text string) map[string]any {
	return map[string]any{
		"type": "message", "role": "user",
		"content": []map[string]string{{"type": "input_text", "text": text}},
	}
}

func functionCall(callID, name, args string) map[string]any {
	return map[string]any{
		"type": "function_call", "call_id": callID, "name": name, "arguments": args,
	}
}

func functionOutput(callID, out string) map[string]any {
	return map[string]any{
		"type": "function_call_output", "call_id": callID, "output": out,
	}
}
