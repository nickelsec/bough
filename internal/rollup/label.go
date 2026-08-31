package rollup

import (
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// Label names a goal in a few words.
//
// Nothing here invents a name. Every label is text somebody already wrote,
// picked in order of how well it describes the work:
//
//  1. A sub-agent brief. When the agent handed work to a sub-agent it wrote a
//     description first, and those are short and specific by nature: "Diagnose
//     PDF text layer accuracy", "Design the keygen tool".
//  2. The opening of the first real request in the sitting. Long enough to be a
//     request rather than a nudge, and it usually states the intent.
//
// A goal with neither is left unnamed rather than given something invented. An
// honest blank reads better than a wrong title.
func Label(turns []agent.Turn) string {
	if brief := firstBrief(turns); brief != "" {
		return brief
	}
	return firstRequest(turns)
}

// firstBrief returns the first sub-agent description in the goal.
func firstBrief(turns []agent.Turn) string {
	for _, t := range turns {
		for _, d := range t.Delegated {
			if d.Description != "" {
				return tidy(d.Description)
			}
		}
	}
	return ""
}

// requestLength is how long a prompt has to be before it reads as a request
// rather than a nudge. Matched to the same measurement the segmenter uses.
const requestLength = 80

// firstRequest returns the opening of the first substantial prompt.
func firstRequest(turns []agent.Turn) string {
	for _, t := range turns {
		if len(t.Text) >= requestLength && !pasted(t.Text) {
			return summarise(t.Text)
		}
	}
	// Every long prompt was pasted output, so take the longest thing the user
	// wrote themselves instead.
	for _, t := range turns {
		if !pasted(t.Text) && len(t.Text) > 0 {
			return summarise(t.Text)
		}
	}
	// Nothing long enough, so fall back to the longest thing said.
	best := ""
	for _, t := range turns {
		if len(t.Text) > len(best) {
			best = t.Text
		}
	}
	return summarise(best)
}

// pastedMarkers appear in text the user copied in rather than wrote: stack
// traces, shell errors, deploy output, harness notices. These prompts are long,
// so they win the "first substantial request" test, but they describe a symptom
// rather than the work. Naming a day after an error message reads as though
// that was the point of the day.
var pastedMarkers = []string{
	"at line:", "traceback", "stack trace", "error:", "exception",
	"+ categoryinfo", "+ fullyqualifiederrorid", "npm err", "panic:",
	"(re-invocation of", "invalid configuration",
}

// pasted reports whether a prompt is mostly something the user copied in.
//
// Keywords catch the obvious cases. Shape catches the rest: terminal output
// arrives as many short lines, and a shell prompt or a path at the very start
// means the first thing in the message is a machine talking.
func pasted(text string) bool {
	t := strings.ToLower(text)
	for _, m := range pastedMarkers {
		if strings.Contains(t, m) {
			return true
		}
	}

	trimmed := strings.TrimSpace(text)
	for _, p := range shellPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return true
		}
	}

	// Console output is many short lines. Prose wraps long or not at all.
	lines := strings.Split(trimmed, "\n")
	if len(lines) >= 4 {
		short := 0
		for _, l := range lines {
			if n := len(strings.TrimSpace(l)); n > 0 && n < 60 {
				short++
			}
		}
		if short*3 >= len(lines)*2 {
			return true
		}
	}
	return false
}

// shellPrefixes start a pasted terminal session rather than a sentence.
var shellPrefixes = []string{"PS ", "$ ", "> ", "C:\\", "D:\\", "/usr/", "~/"}

// labelWidth is roughly how much fits on one line beside a node.
const labelWidth = 64

// summarise trims a prompt to something that fits, cutting at a word boundary
// so a label never ends mid-word.
func summarise(s string) string {
	s = tidy(s)
	if len(s) <= labelWidth {
		return s
	}
	cut := strings.LastIndex(s[:labelWidth], " ")
	if cut < labelWidth/2 {
		cut = labelWidth
	}
	return strings.TrimRight(s[:cut], " ,.:;-") + "..."
}

// tidy collapses the whitespace that voice dictation and pasted text leave
// behind, so labels sit on one line.
func tidy(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Period describes when a goal happened, in the way a person would say it.
func Period(start, end time.Time) string {
	if start.IsZero() {
		return ""
	}
	if end.IsZero() || sameDay(start, end) {
		return start.Format("Mon 2 Jan")
	}
	if start.Year() == end.Year() && start.Month() == end.Month() {
		return start.Format("2") + " to " + end.Format("2 Jan")
	}
	return start.Format("2 Jan") + " to " + end.Format("2 Jan")
}

func sameDay(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}
