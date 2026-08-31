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

// Fields boughs does not model must not break the parse, because the format
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
