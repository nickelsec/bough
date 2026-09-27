package pi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// readFixture reads a fixture's entries, header included.
func readFixture(t *testing.T, name string) []*Entry {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	entries, err := ReadEntries(f)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// mainTurns is the main fixture session, read the way Sessions reads it.
func mainTurns(t *testing.T) []agent.Turn {
	t.Helper()
	entries := readFixture(t, "sessions/main.jsonl")
	return ExtractTurns(entries[1:], entries[0].Cwd)
}

// Every number below is worked out by hand from what make.mjs asked Pi to
// write, not read back from bough, so the test says what the answer should
// be rather than what it was.

func TestMainSessionHasOneTurnPerPrompt(t *testing.T) {
	turns := mainTurns(t)
	want := []string{
		"add a readme and commit it",
		"/skill:review src/main.go please",
		"", // a picture with nothing typed
		"delegate a test run",
		"try another way",
	}
	if len(turns) != len(want) {
		t.Fatalf("got %d turns, want %d", len(turns), len(want))
	}
	for i, w := range want {
		if turns[i].Text != w {
			t.Errorf("turn %d text = %q, want %q", i, turns[i].Text, w)
		}
		if turns[i].At.IsZero() {
			t.Errorf("turn %d has no time", i)
		}
	}
}

// The first turn writes one file, edits another, commits once and fails once.
func TestToolsFilesAndLines(t *testing.T) {
	c := mainTurns(t)[0]
	readme := agent.NormalisePath(`C:\work\app\README.md`)
	main := agent.NormalisePath(`C:\work\app\src\main.go`)

	if c.Tools["write"] != 1 || c.Tools["edit"] != 1 || c.Tools["bash"] != 2 {
		t.Errorf("tools = %v", c.Tools)
	}
	// "@src/main.go" is a relative path with Pi's "@" in front of it.
	if c.Files[readme] != 1 || c.Files[main] != 1 || len(c.Files) != 2 {
		t.Errorf("files = %v", c.Files)
	}
	if c.Edits[readme] != 1 || c.Edits[main] != 1 {
		t.Errorf("edits = %v", c.Edits)
	}
	// The write has no patch, so its two lines of content count. The edit's
	// patch, from Pi's own diff code, removes one line and adds two.
	if c.Lines[readme] != 2 || c.Lines[main] != 3 {
		t.Errorf("lines = %v, want README 2 and main.go 3", c.Lines)
	}
	if c.Errors != 1 {
		t.Errorf("errors = %d, want the one failed commit", c.Errors)
	}
}

// The first commit of a repository prints "(root-commit)" in the middle of the
// line git's hash is read from. The failed one is not a commit.
func TestCommitsAreReadFromWhatGitPrinted(t *testing.T) {
	turns := mainTurns(t)
	got := turns[0].Committed
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1: %+v", len(got), got)
	}
	if got[0].SHA != "85fd4d9" || got[0].Branch != "main" || got[0].Kind != "committed" {
		t.Errorf("commit = %+v", got[0])
	}
	if got[0].At.IsZero() {
		t.Error("commit has no time")
	}

	// The person's own "!git commit --amend" is kept; their failed one is not.
	typed := turns[1].Committed
	if len(typed) != 1 || typed[0].Kind != "amended" || typed[0].SHA != "cd7d300" {
		t.Errorf("typed commits = %+v", typed)
	}
}

// Each model is charged what its replies used, and nothing else.
func TestTokensPerModel(t *testing.T) {
	turns := mainTurns(t)

	luna := turns[0].Models["gpt-5.6-luna"]
	if luna != (agent.Tokens{Input: 1440, Output: 150, CacheRead: 4900}) {
		t.Errorf("turn 0 luna = %+v", luna)
	}

	// Two replies, cache warming, and the compaction's summary, all Anthropic.
	// The failed request is not there: it used nothing.
	opus := turns[1].Models["claude-opus-4-8"]
	want := agent.Tokens{Input: 9015, Output: 800, CacheRead: 13000, CacheWrite: 4000, CacheWriteHour: 4000}
	if opus != want {
		t.Errorf("turn 1 opus = %+v, want %+v", opus, want)
	}
	if len(turns[1].Models) != 1 {
		t.Errorf("turn 1 models = %v", turns[1].Models)
	}

	// Priced under the model asked for, with ":free" kept. The reply names
	// the answering model without it.
	free := turns[2].Models["openrouter/nvidia/nemotron-3-ultra-550b-a55b:free"]
	if free != (agent.Tokens{Input: 700, Output: 60, CacheRead: 100}) {
		t.Errorf("turn 2 models = %v", turns[2].Models)
	}

	for i, turn := range turns {
		var sum agent.Tokens
		for _, m := range turn.Models {
			sum.Add(m)
		}
		if sum != turn.Tokens {
			t.Errorf("turn %d: models sum to %+v, turn says %+v", i, sum, turn.Tokens)
		}
	}
}

// A sub-agent's work lives inside the result the extension hands back, and Pi
// leaves it out of its own totals. bough counts it in the turn that asked.
func TestSubagentWorkIsCounted(t *testing.T) {
	c := mainTurns(t)[3]
	if len(c.Delegated) != 2 {
		t.Fatalf("delegated = %+v", c.Delegated)
	}
	if c.Delegated[0] != (agent.Delegation{Kind: "tester", Description: "run the tests"}) ||
		c.Delegated[1] != (agent.Delegation{Kind: "writer", Description: "write docs"}) {
		t.Errorf("delegated = %+v", c.Delegated)
	}
	// The tester's reply and the writer's summed usage, both on luna.
	if got := c.Models["gpt-5.6-luna"]; got != (agent.Tokens{Input: 450, Output: 130}) {
		t.Errorf("sub-agent luna = %+v", got)
	}
	// The parent's two replies, the tool's nested usage, and the summary of
	// the branch that was left, which was written before anything new was
	// typed and so belongs to this turn.
	if got := c.Models["gpt-5.6-sol"]; got != (agent.Tokens{Input: 1050, Output: 155, CacheRead: 4400}) {
		t.Errorf("parent sol = %+v", got)
	}
	// The tester's failing test run is a failure in this turn.
	if c.Errors != 1 || c.Tools["bash"] != 1 || c.Tools["subagent"] != 1 {
		t.Errorf("errors %d, tools %v", c.Errors, c.Tools)
	}
}

// Compaction, a /tree jump and a branch summary are boundaries Pi recorded. A
// hint marks the last turn before one.
func TestBoundariesAreHinted(t *testing.T) {
	turns := mainTurns(t)
	want := []bool{false, true, false, true, false}
	for i, w := range want {
		if turns[i].SegmentHint != w {
			t.Errorf("turn %d hint = %v, want %v", i, turns[i].SegmentHint, w)
		}
	}
}

// Paths are resolved the way Pi resolves them before a tool touches the disk.
func TestPathsResolveLikePi(t *testing.T) {
	home, _ := os.UserHomeDir()
	win := &walker{cwd: `C:\work\app`}
	nix := &walker{cwd: "/home/u/app"}
	cases := []struct {
		w    *walker
		in   string
		want string
	}{
		{win, "README.md", "c:/work/app/README.md"},
		{win, "@src/x.go", "c:/work/app/src/x.go"},
		{win, `src\x.go`, "c:/work/app/src/x.go"},
		{win, `D:\elsewhere\y.go`, "d:/elsewhere/y.go"},
		{win, "/d/other/y.go", "d:/other/y.go"},
		{win, "/mnt/d/other", "d:/other"},
		{win, "/cygdrive/e/z", "e:/z"},
		{win, "file:///C:/work/app/f", "c:/work/app/f"},
		{win, "../sibling/a", "c:/work/sibling/a"},
		{win, "a\u00a0b.txt", "c:/work/app/a b.txt"},
		{nix, "src/x.go", "/home/u/app/src/x.go"},
		// On anything but Windows "/d/x" is an ordinary directory.
		{nix, "/d/x", "/d/x"},
		{nix, "file:///etc/hosts", "/etc/hosts"},
		{nix, "", ""},
	}
	for _, c := range cases {
		if got := c.w.resolve(c.in); got != c.want {
			t.Errorf("resolve(%q) in %s = %q, want %q", c.in, c.w.cwd, got, c.want)
		}
	}
	if home != "" {
		if got := nix.resolve("~/notes"); got != agent.NormalisePath(home+"/notes") {
			t.Errorf("~ resolved to %q", got)
		}
	}
}

// The patch's own "---" and "+++" headers are not changes, but a removed line
// that starts with "--" is.
func TestPatchLines(t *testing.T) {
	patch := "--- a/q.sql\n+++ b/q.sql\n@@ -1,3 +1,3 @@\n select 1;\n--- note\n+-- better note\n\\ No newline at end of file\n"
	raw, err := json.Marshal(map[string]string{"patch": patch})
	if err != nil {
		t.Fatal(err)
	}
	n, ok := patchLines(raw)
	if !ok || n != 2 {
		t.Errorf("got %d, %v; want 2 changed lines", n, ok)
	}
	if _, ok := patchLines([]byte(`{}`)); ok {
		t.Error("no patch was counted as one")
	}
	if _, ok := patchLines(nil); ok {
		t.Error("no details were counted as a patch")
	}
}

func TestLineCount(t *testing.T) {
	for s, want := range map[string]int{"": 0, "a": 1, "a\n": 1, "a\nb": 2, "a\nb\n": 2, "\n": 1} {
		if got := lineCount(s); got != want {
			t.Errorf("lineCount(%q) = %d, want %d", s, got, want)
		}
	}
}

// line builds one entry of a session file.
func line(typ, id, parent, rest string) string {
	p := "null"
	if parent != "" {
		p = `"` + parent + `"`
	}
	return `{"type":"` + typ + `","id":"` + id + `","parentId":` + p + `,"timestamp":"2026-09-20T10:00:00Z"` + rest + "}\n"
}

func turnsOf(t *testing.T, body string) []agent.Turn {
	t.Helper()
	entries, err := ReadEntries(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	return ExtractTurns(entries, "/w")
}

// Usage from before anyone typed anything, cache warming for instance, is not
// dropped. It goes to the first turn.
func TestEarlyUsageGoesToTheFirstTurn(t *testing.T) {
	turns := turnsOf(t,
		line("usage", "a", "", `,"kind":"cache_warm","provider":"anthropic","model":"claude-opus-4-8","usage":{"input":0,"output":0,"cacheRead":500,"cacheWrite":0}`)+
			line("message", "b", "a", `,"message":{"role":"user","content":"hi","timestamp":1}`))
	if len(turns) != 1 || turns[0].Tokens.CacheRead != 500 || turns[0].Models["claude-opus-4-8"].CacheRead != 500 {
		t.Fatalf("turns = %+v", turns)
	}
}

// Model work nothing names a model for is charged under a name that says so,
// so the cost is refused rather than coming out short.
func TestUnnamedUsageIsNotLeftOffTheBill(t *testing.T) {
	turns := turnsOf(t,
		line("message", "a", "", `,"message":{"role":"user","content":"hi","timestamp":1}`)+
			line("message", "b", "a", `,"message":{"role":"toolResult","toolCallId":"x","toolName":"t","content":[],"isError":false,"usage":{"input":7,"output":3,"cacheRead":0,"cacheWrite":0},"timestamp":2}`))
	if got := turns[0].Models[unknownModel]; got != (agent.Tokens{Input: 7, Output: 3}) {
		t.Errorf("models = %v", turns[0].Models)
	}
}

// Extension messages, system messages and summaries are not prompts.
func TestOnlyAPersonOpensATurn(t *testing.T) {
	turns := turnsOf(t,
		line("message", "a", "", `,"message":{"role":"system","content":"","timestamp":1}`)+
			line("custom_message", "b", "a", `,"customType":"x","content":"injected","display":true`)+
			line("message", "c", "b", `,"message":{"role":"custom","customType":"x","content":"also injected","display":true,"timestamp":2}`)+
			line("message", "d", "c", `,"message":{"role":"user","content":"   ","timestamp":3}`)+
			line("message", "e", "d", `,"message":{"role":"user","content":[{"type":"text","text":"real"}],"timestamp":4}`))
	if len(turns) != 1 || turns[0].Text != "real" {
		t.Errorf("turns = %+v", turns)
	}
}

// A version 1 file has no ids, so nothing can look like a jump in the tree.
func TestAVersionOneFileReadsInOrder(t *testing.T) {
	body := `{"type":"session","id":"s","timestamp":"2026-09-20T10:00:00Z","cwd":"/w"}` + "\n" +
		`{"type":"message","timestamp":"2026-09-20T10:00:01Z","message":{"role":"user","content":"one","timestamp":1}}` + "\n" +
		`{"type":"message","timestamp":"2026-09-20T10:00:02Z","message":{"role":"assistant","content":[{"type":"toolCall","id":"t","name":"edit","arguments":{"path":"a.go","oldText":"x","newText":"y"}}],"provider":"anthropic","model":"claude-opus-4-8","usage":{"input":1,"output":2,"cacheRead":0,"cacheWrite":0},"stopReason":"toolUse","timestamp":2}}` + "\n" +
		`{"type":"compaction","timestamp":"2026-09-20T10:00:03Z","summary":"s","firstKeptEntryIndex":1,"tokensBefore":9}` + "\n" +
		`{"type":"message","timestamp":"2026-09-20T10:00:04Z","message":{"role":"user","content":"two","timestamp":4}}` + "\n"
	turns := turnsOf(t, body)
	if len(turns) != 2 {
		t.Fatalf("got %d turns", len(turns))
	}
	if !turns[0].SegmentHint || turns[1].SegmentHint {
		t.Errorf("hints = %v, %v; only the compaction should mark a boundary", turns[0].SegmentHint, turns[1].SegmentHint)
	}
	// The older edit shape, one replacement at the top level.
	if turns[0].Edits["/w/a.go"] != 1 {
		t.Errorf("edits = %v", turns[0].Edits)
	}
}

// Pi appends as it goes, so the last line of a live session can be cut short.
// Windows line endings and blank lines are ordinary.
func TestReadEntriesTolerance(t *testing.T) {
	body := `{"type":"session","version":3,"id":"s","timestamp":"t","cwd":"/w"}` + "\r\n\r\n" +
		`{"type":"message","id":"a","parentId":null,"timestamp":"t","message":{"role":"user","content":"hi","timestamp":1}}` + "\r\n" +
		`{"type":"message","id":"b","parentId":"a","timestamp":"t","mess`
	entries, err := ReadEntries(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want the header and the one whole message", len(entries))
	}
}

// Counts arrive from JavaScript, where every number is a float.
func TestCountsTolerateFloats(t *testing.T) {
	turns := turnsOf(t,
		line("message", "a", "", `,"message":{"role":"user","content":"hi","timestamp":1}`)+
			line("message", "b", "a", `,"message":{"role":"assistant","content":[],"provider":"openai","model":"gpt-5.6-luna","usage":{"input":12.0,"output":3,"cacheRead":null,"cacheWrite":0,"cost":{"total":0.1}},"stopReason":"stop","timestamp":2}`))
	if got := turns[0].Tokens; got != (agent.Tokens{Input: 12, Output: 3}) {
		t.Errorf("tokens = %+v", got)
	}
}

// Pi prices an Anthropic fallback as the model that answered, and nothing
// else that way.
func TestChargedModel(t *testing.T) {
	cases := []struct {
		m    Message
		want string
	}{
		{Message{Provider: "anthropic", Model: "claude-opus-4-8", ResponseModel: "claude-sonnet-5"}, "claude-sonnet-5"},
		{Message{Provider: "anthropic", Model: "claude-opus-4-8"}, "claude-opus-4-8"},
		{Message{Provider: "openrouter", Model: "x/y:free", ResponseModel: "x/y"}, "x/y:free"},
	}
	for _, c := range cases {
		if got := chargedModel(&c.m); got != c.want {
			t.Errorf("chargedModel(%+v) = %q, want %q", c.m, got, c.want)
		}
	}
}

func TestPromptTextPutsSkillsBack(t *testing.T) {
	cases := map[string]string{
		"<skill name=\"a\" location=\"/s/a/SKILL.md\">\nbody\nmore body\n</skill>\n\ndo it": "/skill:a do it",
		"<skill name=\"a\" location=\"/s/a/SKILL.md\">\nbody\n</skill>":                     "/skill:a",
		"  plain  ":                              "plain",
		"<skill> not really one":                 "<skill> not really one",
		"text mentioning <skill name=\"a\"> mid": "text mentioning <skill name=\"a\"> mid",
	}
	for in, want := range cases {
		if got := promptText(in); got != want {
			t.Errorf("promptText(%q) = %q, want %q", in, got, want)
		}
	}
}
