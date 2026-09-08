package antigravity

import (
	"os"
	"testing"
)

func TestExtractTurnsFromFixture(t *testing.T) {
	f, err := os.Open("testdata/transcript.jsonl")
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
	if turn.Text != "implement cache layer and commit" {
		t.Errorf("unexpected turn text: %q", turn.Text)
	}

	// Tools
	if turn.Tools["view_file"] != 1 {
		t.Errorf("expected 1 view_file, got %d", turn.Tools["view_file"])
	}
	if turn.Tools["write_to_file"] != 1 {
		t.Errorf("expected 1 write_to_file, got %d", turn.Tools["write_to_file"])
	}
	if turn.Tools["run_command"] != 1 {
		t.Errorf("expected 1 run_command, got %d", turn.Tools["run_command"])
	}
	if turn.Tools["invoke_subagent"] != 1 {
		t.Errorf("expected 1 invoke_subagent, got %d", turn.Tools["invoke_subagent"])
	}

	// Files and Edits
	mainGo := normalisePath("/Users/alice/work/api/main.go")
	cacheGo := normalisePath("/Users/alice/work/api/cache.go")
	if turn.Files[mainGo] != 1 {
		t.Errorf("expected 1 touch on main.go, got %d", turn.Files[mainGo])
	}
	if turn.Files[cacheGo] != 1 {
		t.Errorf("expected 1 touch on cache.go, got %d", turn.Files[cacheGo])
	}
	if turn.Edits[cacheGo] != 1 {
		t.Errorf("expected 1 edit on cache.go, got %d", turn.Edits[cacheGo])
	}
	if turn.Lines[cacheGo] != 4 {
		t.Errorf("expected 4 lines on cache.go, got %d", turn.Lines[cacheGo])
	}

	// Commit
	if len(turn.Committed) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(turn.Committed))
	}
	if turn.Committed[0].Kind != "committed" {
		t.Errorf("expected kind 'committed', got %q", turn.Committed[0].Kind)
	}
	if turn.Committed[0].SHA != "9876543" {
		t.Errorf("expected SHA '9876543', got %q", turn.Committed[0].SHA)
	}

	// Delegated subagent
	if len(turn.Delegated) != 1 {
		t.Fatalf("expected 1 delegation, got %d", len(turn.Delegated))
	}
	if turn.Delegated[0].Kind != "Benchmark Tester" {
		t.Errorf("expected kind 'Benchmark Tester', got %q", turn.Delegated[0].Kind)
	}
	if turn.Delegated[0].Description != "Run benchmark suite" {
		t.Errorf("expected description 'Run benchmark suite', got %q", turn.Delegated[0].Description)
	}
}

func TestExtractTurnsErrors(t *testing.T) {
	recs := []*Record{
		{
			StepIndex: 0,
			Source:    "USER_EXPLICIT",
			Type:      "USER_INPUT",
			CreatedAt: "2026-09-08T12:00:00Z",
			Content:   "<USER_REQUEST>\nrun tests\n</USER_REQUEST>",
		},
		{
			StepIndex: 1,
			Source:    "MODEL",
			Type:      "PLANNER_RESPONSE",
			CreatedAt: "2026-09-08T12:00:01Z",
			ToolCalls: []ToolCall{
				{
					Name: "run_command",
					Args: map[string]string{
						"CommandLine": `"go test ./..."`,
					},
				},
			},
		},
		{
			StepIndex: 2,
			Source:    "MODEL",
			Type:      "GENERIC",
			CreatedAt: "2026-09-08T12:00:05Z",
			Content:   "FAIL\nexit status 1\nThe command exited with code 1.",
		},
		{
			StepIndex: 3,
			Source:    "MODEL",
			Type:      "PLANNER_RESPONSE",
			CreatedAt: "2026-09-08T12:00:06Z",
			ToolCalls: []ToolCall{
				{
					Name: "view_file",
					Args: map[string]string{
						"AbsolutePath": `"/Users/alice/missing.go"`,
					},
				},
			},
		},
		{
			StepIndex: 4,
			Source:    "MODEL",
			Type:      "GENERIC",
			CreatedAt: "2026-09-08T12:00:07Z",
			Content:   "Encountered error in tool execution: file not found",
		},
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("expected 1 turn, got %d", len(turns))
	}
	if turns[0].Errors != 2 {
		t.Errorf("expected 2 errors, got %d", turns[0].Errors)
	}
}

func TestPromptCleanMetadata(t *testing.T) {
	r := &Record{
		Type:   "USER_INPUT",
		Source: "USER_EXPLICIT",
		Content: "<USER_REQUEST>\nfix authentication bug\n</USER_REQUEST>\n<ADDITIONAL_METADATA>\nLocal time 2026-09-08\n</ADDITIONAL_METADATA>",
	}
	if !r.IsHumanPrompt() {
		t.Fatal("expected IsHumanPrompt true")
	}
	if r.PromptText() != "fix authentication bug" {
		t.Errorf("expected 'fix authentication bug', got %q", r.PromptText())
	}
}
