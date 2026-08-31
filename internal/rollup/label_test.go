package rollup

import (
	"strings"
	"testing"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// A sub-agent brief was written to describe the work, so it beats guessing from
// the prompts around it.
func TestLabelPrefersASubAgentBrief(t *testing.T) {
	turns := []agent.Turn{
		{Text: "so i was thinking about the whole pdf situation and how the text layer might be wrong somehow"},
		{Delegated: []agent.Delegation{{Kind: "Explore", Description: "Diagnose PDF text layer accuracy"}}},
	}
	if got := Label(turns); got != "Diagnose PDF text layer accuracy" {
		t.Errorf("label = %q, want the brief", got)
	}
}

func TestLabelFallsBackToTheFirstRealRequest(t *testing.T) {
	turns := []agent.Turn{
		{Text: "go for it"},
		{Text: "rework the checkout flow so the discount is applied before tax rather than after it"},
	}
	got := Label(turns)
	if !strings.HasPrefix(got, "rework the checkout flow") {
		t.Errorf("label = %q, want the substantial request", got)
	}
}

// Voice dictation and pasted text arrive full of line breaks and runs of
// spaces, and a label has to sit on one line.
func TestLabelCollapsesWhitespace(t *testing.T) {
	turns := []agent.Turn{{Text: "make   the\n\n  nav   bar\tsmaller and tighten the spacing around the logo please"}}
	got := Label(turns)
	if strings.ContainsAny(got, "\n\t") || strings.Contains(got, "  ") {
		t.Errorf("label = %q, still has stray whitespace", got)
	}
}

// A label must never end mid-word.
func TestLabelCutsAtAWordBoundary(t *testing.T) {
	long := "rewrite the entire authentication subsystem so that refresh tokens rotate correctly and sessions survive a restart"
	got := Label([]agent.Turn{{Text: long}})

	if len(got) > labelWidth+3 {
		t.Errorf("label is %d chars, too long: %q", len(got), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("a trimmed label should say so: %q", got)
	}
	trimmed := strings.TrimSuffix(got, "...")
	if !strings.HasPrefix(long, trimmed) {
		t.Errorf("label %q is not a prefix of the prompt", got)
	}
}

// Better to say nothing than to invent a name.
func TestLabelOnNothing(t *testing.T) {
	if got := Label(nil); got != "" {
		t.Errorf("label = %q, want empty", got)
	}
}

func TestPeriodReadsLikeAPerson(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 8, day, 10, 0, 0, 0, time.UTC) }

	tests := []struct {
		start, end time.Time
		want       string
	}{
		{d(3), d(3), "Mon 3 Aug"},
		{d(3), d(5), "3 to 5 Aug"},
		{d(30), time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC), "30 Aug to 2 Sep"},
	}
	for _, tc := range tests {
		if got := Period(tc.start, tc.end); got != tc.want {
			t.Errorf("Period = %q, want %q", got, tc.want)
		}
	}
	if got := Period(time.Time{}, time.Time{}); got != "" {
		t.Errorf("no timestamps should give no period, got %q", got)
	}
}

// Long prompts are often pasted output rather than a request. They win the
// length test and then name a day after an error message.
func TestLabelSkipsPastedOutput(t *testing.T) {
	tests := []struct {
		name  string
		turns []agent.Turn
		want  string
	}{
		{
			name: "shell session",
			turns: []agent.Turn{
				{Text: "PS D:\\proj> make lint && make test\nAt line:1 char:11\n+ make lint && make test\n+           ~~"},
				{Text: "sort out the linter so the build stops failing on every single run"},
			},
			want: "sort out the linter",
		},
		{
			name: "stack trace",
			turns: []agent.Turn{
				{Text: "Traceback (most recent call last):\n  File \"x.py\", line 1\n    boom\nValueError: bad"},
				{Text: "the importer falls over whenever a row is missing its identifier column"},
			},
			want: "the importer falls over",
		},
		{
			name: "console output shape",
			turns: []agent.Turn{
				{Text: "build ok\nlint ok\ntest ok\ndeploy ok\nall green\ndone in 4s"},
				{Text: "now wire the deploy step into the release workflow so it runs on a tag"},
			},
			want: "now wire the deploy step",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Label(tc.turns)
			if !strings.HasPrefix(got, tc.want) {
				t.Errorf("label = %q, want it to start with %q", got, tc.want)
			}
		})
	}
}

// When everything in the goal was pasted, say something rather than nothing.
func TestLabelWhenEverythingWasPasted(t *testing.T) {
	turns := []agent.Turn{
		{Text: "PS D:\\proj> go build ./...\nsome output here that goes on"},
	}
	if got := Label(turns); got == "" {
		t.Error("expected a fallback label rather than nothing")
	}
}
