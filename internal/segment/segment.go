// Package segment groups a session's turns into tasks.
//
// A task is a run of turns working towards one thing. Sessions do not hand this
// to you. On the history this was built against, a single session ran for nine
// days and covered a dozen unrelated pieces of work, so the boundaries have to
// be recovered.
//
// Everything here works from normalised turns. Nothing in this package knows
// which agent the history came from.
package segment

import (
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// Reason records why a boundary was drawn. Keeping these makes the output
// reviewable, since a cut a person disagrees with can be traced to the signal
// that caused it.
type Reason string

const (
	// ReasonGap means the user stopped working for a while.
	ReasonGap Reason = "gap"

	// ReasonCompaction means the agent compacted its context here, which is a
	// judgement the agent itself made that the thread had moved on.
	ReasonCompaction Reason = "compaction"

	// ReasonFiles means the prompt started touching a different part of the tree.
	ReasonFiles Reason = "files"

	// ReasonTopic means the prompt stopped sharing vocabulary with the ones before it.
	ReasonTopic Reason = "topic"
)

// Task is a run of turns treated as one piece of work.
type Task struct {
	Turns []agent.Turn

	// Reasons is why this task began. The first task of a session has none.
	Reasons []Reason
}

// Options tune where boundaries fall.
//
// The defaults were fitted against one developer's history, so they are exposed
// rather than baked in. See docs/tuning.md before changing them.
type Options struct {
	// Gap is how long a pause has to be before it ends a task on its own.
	Gap time.Duration

	// CompactionGap is the pause required alongside a compaction. Compaction
	// during continuous work usually means the context filled up, not that the
	// user moved on.
	CompactionGap time.Duration

	// FileSimilarity is the overlap below which two file sets count as
	// different work, between 0 and 1.
	FileSimilarity float64

	// TopicSimilarity is the word overlap below which two prompts count as
	// different subjects, between 0 and 1.
	TopicSimilarity float64

	// MinFilesForSignal is how many files each side needs before file overlap
	// is trusted. Comparing one file against two is noise.
	MinFilesForSignal int

	// LongRun is the number of turns after which a single weak signal is enough
	// to cut. Without this, work with no file locality runs on forever.
	LongRun int
}

// DefaultOptions are the fitted defaults.
func DefaultOptions() Options {
	return Options{
		Gap:               45 * time.Minute,
		CompactionGap:     20 * time.Minute,
		FileSimilarity:    0.10,
		TopicSimilarity:   0.08,
		MinFilesForSignal: 3,
		LongRun:           10,
	}
}

// Split groups turns into tasks.
//
// A pause or a compaction is enough on its own, since both are the user or the
// agent saying the thread ended. The content signals are weaker and need to
// agree with something else before they cut, because acting on either alone
// shatters iterative work into single prompts. On one UI-heavy session, cutting
// on file divergence alone produced 52 tasks out of 154 turns, most of them a
// single tweak to the same page.
func Split(turns []agent.Turn, opt Options) []Task {
	if len(turns) == 0 {
		return nil
	}

	var tasks []Task
	cur := []agent.Turn{turns[0]}
	var curReasons []Reason
	window := newWindow()
	window.add(turns[0])

	for i := 1; i < len(turns); i++ {
		t := turns[i]
		prev := turns[i-1]

		var strong, weak []Reason
		gap := interval(prev, t)

		if gap >= opt.Gap {
			strong = append(strong, ReasonGap)
		}
		if prev.SegmentHint && gap >= opt.CompactionGap {
			strong = append(strong, ReasonCompaction)
		}

		// Content signals only apply to prompts that say something. A two word
		// follow up carries no evidence either way.
		if isSubstantive(t.Text) {
			if len(window.files) >= opt.MinFilesForSignal && len(t.Files) >= opt.MinFilesForSignal {
				if jaccard(window.files, t.Files) < opt.FileSimilarity {
					weak = append(weak, ReasonFiles)
				}
			}
			if len(cur) >= 2 && window.words != nil {
				if overlap(window.words, wordsOf(t.Text)) < opt.TopicSimilarity {
					weak = append(weak, ReasonTopic)
				}
			}
		}

		cut := len(strong) > 0 ||
			len(weak) >= 2 ||
			(len(weak) >= 1 && len(cur) >= opt.LongRun)

		if cut {
			tasks = append(tasks, Task{Turns: cur, Reasons: curReasons})
			cur = nil
			curReasons = append(strong, weak...)
			window = newWindow()
		}
		cur = append(cur, t)
		window.add(t)
	}
	return append(tasks, Task{Turns: cur, Reasons: curReasons})
}

// interval is the time between two turns, or zero when either lacks a timestamp.
func interval(a, b agent.Turn) time.Duration {
	if a.At.IsZero() || b.At.IsZero() {
		return 0
	}
	return b.At.Sub(a.At)
}

// window holds a decaying view of the recent turns, so comparisons are against
// what the user has been doing lately rather than the whole task.
type window struct {
	turns []agent.Turn
	files map[string]int
	words map[string]bool
}

const windowSize = 8

func newWindow() *window {
	return &window{files: map[string]int{}, words: map[string]bool{}}
}

func (w *window) add(t agent.Turn) {
	w.turns = append(w.turns, t)
	if len(w.turns) > windowSize {
		w.turns = w.turns[len(w.turns)-windowSize:]
		w.files = map[string]int{}
		w.words = map[string]bool{}
		for _, x := range w.turns {
			w.absorb(x)
		}
		return
	}
	w.absorb(t)
}

func (w *window) absorb(t agent.Turn) {
	for f := range t.Files {
		w.files[f]++
	}
	if isSubstantive(t.Text) {
		for _, word := range wordsOf(t.Text) {
			w.words[word] = true
		}
	}
}

// jaccard is the overlap between two sets of files.
func jaccard(a map[string]int, b map[string]int) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	shared := 0
	for k := range b {
		if _, ok := a[k]; ok {
			shared++
		}
	}
	union := len(a) + len(b) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

// overlap is the share of a prompt's words already seen in the window.
func overlap(seen map[string]bool, words []string) float64 {
	if len(seen) == 0 || len(words) == 0 {
		return 1 // no evidence, so do not cut
	}
	shared := 0
	for _, w := range words {
		if seen[w] {
			shared++
		}
	}
	return float64(shared) / float64(len(words))
}

// substantiveLength is how long a prompt has to be before its wording is worth
// comparing.
//
// Chosen from the length distribution of real prompts rather than picked. Across
// 719 prompts the median was 50 characters, and everything below about 80 was
// dominated by continuations: "whats next on out action item?", "yes this works
// but still it misses out some names". New requests reliably run longer.
const substantiveLength = 80

func isSubstantive(text string) bool {
	if len(text) < substantiveLength {
		return false
	}
	return !continuation(text)
}

// continuationOpeners are how people say carry on. A prompt starting this way
// is following the current thread, whatever else it contains.
var continuationOpeners = []string{
	"go for it", "lets ", "let's ", "do it", "yes", "yep", "yeah", "ok", "okay",
	"continue", "next", "proceed", "commit", "push", "what", "sure", "cool",
	"perfect", "thanks", "good", "nice", "fix", "now ", "also ", "and ",
	"make ", "move ", "increase", "decrease", "reduce", "remove", "change",
	"add ", "put ", "give ", "take ", "set ", "use ", "try ", "no ", "more ",
}

func continuation(text string) bool {
	t := strings.ToLower(strings.TrimSpace(text))
	for _, p := range continuationOpeners {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return false
}
