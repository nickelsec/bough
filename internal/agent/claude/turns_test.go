package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsSyntheticRecognisesHarnessText(t *testing.T) {
	synthetic := []string{
		"<system-reminder>do a thing</system-reminder>",
		"[Request interrupted by user]",
		"[Image: original 2880x166, displayed at 2000x115]",
		"This session is being continued from a previous conversation",
		"Base directory for this skill: /skills/humanizer",
		"",
		"   ",
	}
	for _, s := range synthetic {
		if !isSynthetic(s) {
			t.Errorf("isSynthetic(%q) = false, want true", s)
		}
	}

	real := []string{
		"add a --json flag to the cli",
		"now make it 50px",
		"why is the hero section crowded?",
	}
	for _, s := range real {
		if isSynthetic(s) {
			t.Errorf("isSynthetic(%q) = true, want false", s)
		}
	}
}

// The same file appears with different drive letter casing and separators
// across a session. Grouping by file only works if those collapse together.
func TestNormalisePathCollapsesCasingAndSeparators(t *testing.T) {
	same := []string{
		`D:\proj\src\main.go`,
		`d:\proj\src\main.go`,
		`d:/proj/src/main.go`,
		`d:/proj//src/main.go`,
	}
	want := normalisePath(same[0])
	for _, p := range same[1:] {
		if got := normalisePath(p); got != want {
			t.Errorf("normalisePath(%q) = %q, want %q", p, got, want)
		}
	}
	if normalisePath("") != "" {
		t.Error("empty path should stay empty")
	}
}

func TestExtractTurnsAttributesWorkToThePrompt(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"add a flag"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Edit","input":{"file_path":"D:/proj/main.go"}},` +
			`{"type":"tool_use","name":"Read","input":{"file_path":"d:/proj/main.go"}}]}}`,
		`{"uuid":"3","type":"user","message":{"role":"user","content":[{"type":"tool_result","is_error":true}]}}`,
		`{"uuid":"4","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"now write the docs"}]}}`,
		`{"uuid":"5","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Write","input":{"file_path":"/proj/README.md"}}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Text != "add a flag" {
		t.Errorf("first prompt = %q", turns[0].Text)
	}
	if turns[0].Tools["Edit"] != 1 || turns[0].Tools["Read"] != 1 {
		t.Errorf("tools = %v, want one Edit and one Read", turns[0].Tools)
	}
	// Edit and Read touched the same file written two ways, so it counts twice
	// against one path and only the edit is recorded as a change.
	if len(turns[0].Files) != 1 {
		t.Errorf("files = %v, want a single normalised path", turns[0].Files)
	}
	if len(turns[0].Edits) != 1 {
		t.Errorf("edits = %v, want one", turns[0].Edits)
	}
	if turns[0].Errors != 1 {
		t.Errorf("errors = %d, want 1", turns[0].Errors)
	}
	if turns[1].Tools["Write"] != 1 {
		t.Errorf("second turn tools = %v", turns[1].Tools)
	}
}

func TestExtractTurnsSkipsSyntheticPrompts(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"real request"}]}}`,
		`{"uuid":"2","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}`,
		`{"uuid":"3","type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{}}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	// Work that followed the interruption belongs to the request that prompted it.
	if turns[0].Tools["Bash"] != 1 {
		t.Errorf("tools = %v, want the Bash call credited to the real prompt", turns[0].Tools)
	}
}

func TestExtractTurnsRecordsCompactionHint(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"first"}]}}`,
		`{"uuid":"2","type":"system","subtype":"compact_boundary"}`,
		`{"uuid":"3","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"second"}]}}`,
	}
	recs, _ := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	turns := ExtractTurns(recs)

	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if !turns[0].SegmentHint {
		t.Error("compaction after the first turn was not recorded")
	}
	if turns[1].SegmentHint {
		t.Error("second turn should carry no hint")
	}
}

// The counts here were established by hand against this machine's history
// during design. They are the check that noise filtering did not drift.
func TestRealCorpusTurnCounts(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	fp := filepath.Join(home, ".claude", "projects", "d--chaff-app",
		"80fe749b-bfd9-4329-a3e9-b4cb425e920e.jsonl")
	f, err := os.Open(fp)
	if err != nil {
		t.Skip("this machine does not have the reference session")
	}
	defer f.Close()

	recs, err := ReadRecords(f)
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	// Reading this session line by line reports 375 prompts. Collapsing the
	// replay and dropping harness text leaves what the user actually typed.
	if len(turns) < 120 || len(turns) > 175 {
		t.Errorf("got %d turns, expected roughly 146; noise filtering has drifted", len(turns))
	}
	t.Logf("reference session: %d records, %d human turns", len(recs), len(turns))
}

// Sub-agent work carries a description written at the time, which is a better
// label than anything that could be inferred from the prompts around it.
func TestExtractTurnsCapturesDelegations(t *testing.T) {
	lines := []string{
		`{"uuid":"1","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"research the options"}]}}`,
		`{"uuid":"2","type":"assistant","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Task","input":{"subagent_type":"Explore","description":"Research PDF redaction stack"}},` +
			`{"type":"tool_use","name":"Task","input":{"subagent_type":"Plan","description":"Design the architecture"}}]}}`,
	}
	recs, err := ReadRecords(strings.NewReader(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	turns := ExtractTurns(recs)

	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	if len(turns[0].Delegated) != 2 {
		t.Fatalf("got %d delegations, want 2", len(turns[0].Delegated))
	}
	if turns[0].Delegated[0].Kind != "Explore" {
		t.Errorf("kind = %q, want Explore", turns[0].Delegated[0].Kind)
	}
	if turns[0].Delegated[0].Description != "Research PDF redaction stack" {
		t.Errorf("description = %q", turns[0].Delegated[0].Description)
	}
}
