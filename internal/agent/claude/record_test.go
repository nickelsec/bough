package claude

import (
	"strings"
	"testing"
)

// Message content arrives as a list of blocks most of the time and as a bare
// string occasionally. Both shapes are real, so both have to decode.
func TestContentAcceptsBothShapes(t *testing.T) {
	tests := []struct {
		name       string
		line       string
		wantText   string
		wantBlocks int
	}{
		{
			name:     "bare string",
			line:     `{"uuid":"a","type":"user","message":{"role":"user","content":"hello"}}`,
			wantText: "hello",
		},
		{
			name:       "list of blocks",
			line:       `{"uuid":"a","type":"user","message":{"role":"user","content":[{"type":"text","text":"hello"}]}}`,
			wantBlocks: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs, err := ReadRecords(strings.NewReader(tc.line))
			if err != nil {
				t.Fatal(err)
			}
			if len(recs) != 1 || recs[0].Message == nil {
				t.Fatalf("record did not parse: %+v", recs)
			}
			got := recs[0].Message.Content
			if got.Text != tc.wantText {
				t.Errorf("text = %q, want %q", got.Text, tc.wantText)
			}
			if len(got.Blocks) != tc.wantBlocks {
				t.Errorf("blocks = %d, want %d", len(got.Blocks), tc.wantBlocks)
			}
		})
	}
}

// Fields bough does not model must not break the parse, because the format
// gains and loses keys between Claude Code releases.
func TestUnknownFieldsAreIgnored(t *testing.T) {
	line := `{"uuid":"a","type":"user","somethingNew":{"nested":[1,2,3]},"alsoNew":42}`
	recs, err := ReadRecords(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].UUID != "a" {
		t.Fatalf("got %+v, want one record with uuid a", recs)
	}
}

// toolUseResult is an object for some tools and a bare string for others.
// Declaring it as an object only made the whole line fail to parse, which
// silently dropped 85 of the 340 records in the replay fixture. A record lost
// this way leaves no trace, so it is worth pinning both shapes.
func TestToolUseResultAcceptsBothShapes(t *testing.T) {
	lines := strings.Join([]string{
		`{"uuid":"a","type":"user","toolUseResult":"just some text"}`,
		`{"uuid":"b","type":"user","toolUseResult":{"stdout":"ok"}}`,
		`{"uuid":"c","type":"user","toolUseResult":{"gitOperation":{"commit":{"sha":"d0a65cc","kind":"committed","branch":"main"}}}}`,
	}, "\n")

	recs, err := ReadRecords(strings.NewReader(lines))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3 (a string result must not lose the line)", len(recs))
	}

	if c := recs[0].Commit(); c != nil {
		t.Errorf("string result reported a commit: %+v", c)
	}
	if c := recs[1].Commit(); c != nil {
		t.Errorf("result without a git operation reported a commit: %+v", c)
	}

	c := recs[2].Commit()
	if c == nil {
		t.Fatal("no commit read from a record that carries one")
	}
	if c.SHA != "d0a65cc" || c.Kind != "committed" || c.Branch != "main" {
		t.Errorf("got %+v, want sha d0a65cc, kind committed, branch main", *c)
	}
}

// A commit with no hash is not checkable against a repository, which is the
// only reason to record one at all.
func TestCommitNeedsAHash(t *testing.T) {
	line := `{"uuid":"a","type":"user","toolUseResult":{"gitOperation":{"commit":{"kind":"committed"}}}}`
	recs, err := ReadRecords(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if c := recs[0].Commit(); c != nil {
		t.Errorf("got %+v, want nil for a commit with no sha", c)
	}
}

func TestIsHumanPrompt(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{
			name: "human turn as text blocks",
			line: `{"uuid":"a","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"text","text":"add a flag"}]}}`,
			want: true,
		},
		{
			name: "human turn as plain string",
			line: `{"uuid":"a","type":"user","promptId":"p1","message":{"role":"user","content":"add a flag"}}`,
			want: true,
		},
		{
			// Tool output is filed under type user too, and on the real corpus
			// it outnumbered genuine prompts by more than thirteen to one.
			name: "tool result wearing a user record",
			line: `{"uuid":"a","type":"user","promptId":"p1","message":{"role":"user","content":[{"type":"tool_result","is_error":false}]}}`,
			want: false,
		},
		{
			name: "sub-agent turn",
			line: `{"uuid":"a","type":"user","promptId":"p1","isSidechain":true,"message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
			want: false,
		},
		{
			name: "no prompt id",
			line: `{"uuid":"a","type":"user","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
			want: false,
		},
		{
			name: "assistant turn",
			line: `{"uuid":"a","type":"assistant","promptId":"p1","message":{"role":"assistant","content":[{"type":"text","text":"hi"}]}}`,
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			recs, err := ReadRecords(strings.NewReader(tc.line))
			if err != nil {
				t.Fatal(err)
			}
			if got := recs[0].IsHumanPrompt(); got != tc.want {
				t.Errorf("IsHumanPrompt() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsCompactBoundary(t *testing.T) {
	line := `{"uuid":"a","type":"system","subtype":"compact_boundary"}`
	recs, err := ReadRecords(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if !recs[0].IsCompactBoundary() {
		t.Error("compact boundary not recognised")
	}
}

// A malformed timestamp should give back the zero time rather than stopping the
// session from loading.
func TestTimeToleratesBadTimestamps(t *testing.T) {
	line := `{"uuid":"a","type":"user","timestamp":"not a time"}`
	recs, err := ReadRecords(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	if !recs[0].Time().IsZero() {
		t.Error("expected zero time for an unparseable timestamp")
	}
}

func TestPromptTextJoinsTextBlocksOnly(t *testing.T) {
	line := `{"uuid":"a","type":"user","promptId":"p1","message":{"role":"user","content":[` +
		`{"type":"text","text":"first"},` +
		`{"type":"tool_result","is_error":false},` +
		`{"type":"text","text":"second"}]}}`

	recs, err := ReadRecords(strings.NewReader(line))
	if err != nil {
		t.Fatal(err)
	}
	want := "first\nsecond"
	if got := recs[0].PromptText(); got != want {
		t.Errorf("PromptText() = %q, want %q", got, want)
	}
}

// An edit says how much it changed on its result, not on the call. Both shapes
// have to be read: a diff carries a patch, a write to a new file has nothing to
// diff against and carries what it wrote.
func TestChangedCountsBothShapes(t *testing.T) {
	patch := &ToolUseResult{StructuredPatch: []Hunk{
		{Lines: []string{" kept", "-gone", "+added", " kept"}},
		{Lines: []string{"-also gone"}},
	}}
	if got := patch.Changed(); got != 3 {
		t.Errorf("patch: got %d changed lines, want 3", got)
	}

	// A rewrite is a removal and an addition. Both count: moving a block
	// around a file is work whether or not the totals come out even.
	written := &ToolUseResult{Content: "one\ntwo\nthree"}
	if got := written.Changed(); got != 3 {
		t.Errorf("written file: got %d, want 3", got)
	}

	edited := &ToolUseResult{NewString: "a\nb"}
	if got := edited.Changed(); got != 2 {
		t.Errorf("new string: got %d, want 2", got)
	}

	// A patch wins over the written text, since it is the more exact of the
	// two and both arrive together on an edit.
	both := &ToolUseResult{
		StructuredPatch: []Hunk{{Lines: []string{"+one"}}},
		Content:         "a\nb\nc\nd\ne",
	}
	if got := both.Changed(); got != 1 {
		t.Errorf("patch and content: got %d, want 1", got)
	}

	if (&ToolUseResult{}).Changed() != 0 {
		t.Error("an empty result changed nothing")
	}
	var none *ToolUseResult
	if none.Changed() != 0 {
		t.Error("a missing result changed nothing")
	}
}
