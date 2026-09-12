package server

import (
	"regexp"
	"strings"
	"testing"
)

// Every filter in the rail is wired at both ends.
//
// A tab carries a data-panel name, and three other things have to agree with
// it: the panel it opens, the Clear button inside that panel, and the entry in
// the live map that decides whether the tab shows a dot. Nothing in the page
// ties those together, and the failure is not quiet. marks() reaches for
// f-<name>-clear on every refilter, so a panel added without one throws on the
// first keystroke and the whole filter rail stops responding.
//
// That is exactly what a fourth filter nearly did: the Clear buttons were
// driven by a list of names written out a second time, which had no reason to
// stay in step with the panels themselves.
func TestEveryRailPanelIsWiredBothWays(t *testing.T) {
	page, err := assets.ReadFile("index.html")
	if err != nil {
		t.Fatal(err)
	}
	script, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)
	js := withoutComments(string(script))

	tabs := regexp.MustCompile(`data-panel="([a-z]+)"`).FindAllStringSubmatch(html, -1)
	if len(tabs) < 4 {
		t.Fatalf("found %d rail tabs, want at least the four filters", len(tabs))
	}

	// The live map has to be read on its own. Every filter also has a field on
	// the filters object spelled the same way, and searching the whole script
	// for the name finds that instead: a filter dropped from the live map
	// would go unnoticed, which is the one thing this is here to catch.
	live := liveMap(js)

	for _, tab := range tabs {
		name := tab[1]
		if !strings.Contains(html, `id="p-`+name+`"`) {
			t.Errorf("the %s tab opens a panel that is not in the page", name)
		}
		if !strings.Contains(html, `id="f-`+name+`-clear"`) {
			t.Errorf("the %s panel has no Clear button, so marks() throws on the first keystroke", name)
		}
		// The live map in marks() is what lights the tab's dot. A filter
		// missing from it is a filter you cannot see is on.
		if !regexp.MustCompile(`\b` + name + `:\s`).MatchString(live) {
			t.Errorf("%s is not in the live map, so its tab never shows a dot", name)
		}
	}
}

// liveMap returns the body of the live map in marks(), the object that decides
// which rail tabs are showing a dot.
func liveMap(js string) string {
	const opens = "var live = {"
	at := strings.Index(js, opens)
	if at < 0 {
		return ""
	}
	rest := js[at+len(opens):]
	end := strings.Index(rest, "}")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// The search ring is declared before the hover highlight.
//
// Both are single class selectors on .prompt-node, so they carry the same
// specificity and source order is the only thing that separates them. Hover
// has to win: a ring is a standing state read across the whole diagram, while
// lit is the one node under the pointer, and a search whose rings went dark
// wherever the mouse landed would be worse than no rings at all.
//
// Nothing about the stylesheet says this out loud. Moving either block would
// break it silently and the page would still render, which is why it is pinned
// here rather than left to be noticed.
func TestTheSearchRingIsDeclaredBeforeTheHoverHighlight(t *testing.T) {
	b, err := assets.ReadFile("bough.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(b)

	found := strings.Index(css, ".prompt-node.found")
	lit := strings.Index(css, ".prompt-node.lit")

	if found < 0 {
		t.Fatal("nothing marks a prompt that matched the search")
	}
	if lit < 0 {
		t.Fatal("nothing marks the prompt under the pointer")
	}
	if found > lit {
		t.Error("the search ring is declared after the hover highlight, so hovering a ringed prompt leaves it ringed instead of lighting it")
	}
}

// Prompt text is lowercased once, not on every keystroke.
//
// The text searched is the untrimmed thing the person typed, and a pasted
// stack trace or a dictated paragraph runs to kilobytes. Lowercasing the whole
// corpus inside the input handler allocates all of it again for every
// character, and someone typing at a normal speed does that eight times a
// second.
//
// Measured on the largest history here, 324 prompts and 116KB of prose: one
// pass costs under a millisecond and is paid once, on the first search rather
// than at boot, since most people open a diagram and never search it.
func TestPromptTextIsLoweredOnceRatherThanPerKeystroke(t *testing.T) {
	b, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	src := withoutComments(string(b))

	if strings.Contains(src, ".text.toLowerCase()") {
		t.Error("prompt text is lowercased inside the search, which reallocates the whole corpus on every keystroke")
	}
	if !strings.Contains(src, "function readyHay") {
		t.Error("nothing builds the lowered copy the search reads")
	}
}
