package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The money on the page is arithmetic, so it is checked like arithmetic.
//
// Node runs it against the shipped bough.js rather than a copy, for the same
// reason the layout check does: a reimplementation in Go would test the
// reimplementation. The cases that matter are the ones where a wrong answer
// still looks like an answer, chiefly work that has no figure at all being
// treated as work that cost nothing.
func TestSpendHelpersHoldUp(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}

	dir := t.TempDir()
	page, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	js := filepath.Join(dir, "bough.js")
	if err := os.WriteFile(js, page, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "check.js")
	if err := os.WriteFile(script, []byte(spendCheck), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("node", script, js).CombinedOutput()
	if err != nil {
		t.Fatalf("spend checks failed:\n%s", out)
	}
	t.Logf("\n%s", out)
}

// No two functions in the page may share a name.
//
// The page is one long closure, so both declarations hoist and the later one
// silently replaces the earlier. What breaks is not the new code that took the
// name but the old caller, which now calls something else entirely and is
// nowhere near the change. This has happened twice: once with a var shadowed
// inside a loop, and once when a figure-reading helper was called "measure",
// a name the zoom anchors already had.
func TestNoTwoFunctionsShareAName(t *testing.T) {
	b, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	// Only the ones at the closure's own level, which in this file is two
	// spaces in. A function nested inside another shadows rather than
	// replaces, and that is ordinary: several handlers have their own show
	// or toggle and never meet.
	const own = "  function "
	seen := map[string]int{}
	for _, line := range strings.Split(string(b), "\n") {
		if !strings.HasPrefix(line, own) || strings.HasPrefix(line, own+" ") {
			continue
		}
		name := line[len(own):]
		if i := strings.Index(name, "("); i > 0 {
			seen[name[:i]]++
		}
	}
	for name, n := range seen {
		if n > 1 {
			t.Errorf("%q is declared %d times: the later one replaces the earlier and breaks its callers", name, n)
		}
	}
}

// A figure a reader can see has to come from the graph rather than be worked
// out on the page.
//
// The rates live in Go, in one generated table. A page that multiplied tokens
// by a rate of its own would be a second copy of that table, free to drift
// from the first, and the drift would show as a dollar figure that disagrees
// with the same figure in the terminal.
func TestThePageDoesNotPriceAnything(t *testing.T) {
	b, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	// The shapes a rate table takes: a per-token figure is a very small
	// number, so it is written in exponent form or with a run of zeroes.
	for _, sign := range []string{"e-06", "e-05", "e-07", "0.000001", "cost_per_token"} {
		if strings.Contains(src, sign) {
			t.Errorf("bough.js looks like it holds a price rate (%q): pricing belongs in internal/price", sign)
		}
	}
}
