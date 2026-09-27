package pi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// session builds a Pi session file from its entries, header first.
func session(cwd, parent string, entries ...string) string {
	h := `{"type":"session","version":3,"id":"s","timestamp":"2026-09-20T10:00:00Z","cwd":` + quote(cwd)
	if parent != "" {
		h += `,"parentSession":` + quote(parent)
	}
	return h + "}\n" + strings.Join(entries, "")
}

func user(id, parent, text string) string {
	return line("message", id, parent, `,"message":{"role":"user","content":`+quote(text)+`,"timestamp":1}`)
}

func reply(id, parent, calls string, input int) string {
	return line("message", id, parent, `,"message":{"role":"assistant","content":[`+calls+`],"provider":"anthropic","model":"claude-opus-4-8",`+
		`"usage":{"input":`+itoa(input)+`,"output":1,"cacheRead":0,"cacheWrite":0},"stopReason":"toolUse","timestamp":2}`)
}

func toolCall(id, name, args string) string {
	return `{"type":"toolCall","id":"` + id + `","name":"` + name + `","arguments":` + args + `}`
}

func toolResult(id, parent, call, name, text string, isError bool, details string) string {
	e := "false"
	if isError {
		e = "true"
	}
	d := ""
	if details != "" {
		d = `,"details":` + details
	}
	return line("message", id, parent, `,"message":{"role":"toolResult","toolCallId":"`+call+`","toolName":"`+name+`","content":[{"type":"text","text":`+quote(text)+`}]`+d+`,"isError":`+e+`,"timestamp":3}`)
}

func itoa(n int) string {
	b := []byte{}
	if n == 0 {
		return "0"
	}
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

// Parallel calls come back in whatever order they finish, and a result can
// land after the person has already typed the next prompt. Each settles
// against its own call and the turn that made it.
func TestResultsSettleAgainstTheirOwnCall(t *testing.T) {
	turns := turnsOf(t, strings.Join([]string{
		user("a", "", "commit both"),
		reply("b", "a", toolCall("c1", "bash", `{"command":"git commit -m one"}`)+","+
			toolCall("c2", "bash", `{"command":"cd sub && git commit -m two"}`), 10),
		// The second finishes first.
		toolResult("r2", "b", "c2", "bash", "[main 2222222] two", false, ""),
		user("d", "r2", "while that runs, something else"),
		// The first comes back after the next prompt, and failed.
		toolResult("r1", "d", "c1", "bash", "nothing to commit\n\nCommand exited with code 1", true, ""),
	}, ""))
	if len(turns) != 2 {
		t.Fatalf("got %d turns", len(turns))
	}
	got := turns[0].Committed
	if len(got) != 1 || got[0].SHA != "2222222" || got[0].Dir != "sub" {
		t.Errorf("first turn's commits = %+v, want only the one that landed, made in sub", got)
	}
	if len(turns[1].Committed) != 0 {
		t.Errorf("a late result was credited to the turn it arrived in: %+v", turns[1].Committed)
	}
	// The failure is counted where it arrived: a failure is something that
	// happened then, which is how every reader counts it.
	if turns[1].Errors != 1 {
		t.Errorf("errors = %d, %d", turns[0].Errors, turns[1].Errors)
	}
}

// A commit made quietly prints no hash. It is still a commit, matched to the
// repository by time instead.
func TestAQuietCommitHasNoHash(t *testing.T) {
	turns := turnsOf(t, strings.Join([]string{
		user("a", "", "commit"),
		reply("b", "a", toolCall("c1", "powershell", `{"command":"git commit -q -m quiet"}`), 10),
		toolResult("r1", "b", "c1", "powershell", "", false, ""),
	}, ""))
	got := turns[0].Committed
	if len(got) != 1 || got[0].SHA != "" || got[0].At.IsZero() {
		t.Errorf("commits = %+v", got)
	}
}

// An edit that failed changed nothing. It still counts as an edit, since the
// attempt was made, but it adds no lines.
func TestAFailedEditAddsNoLines(t *testing.T) {
	turns := turnsOf(t, strings.Join([]string{
		user("a", "", "change it"),
		reply("b", "a", toolCall("c1", "edit", `{"path":"a.go","edits":[{"oldText":"x","newText":"y\nz"}]}`), 10),
		toolResult("r1", "b", "c1", "edit", "Could not find the exact text", true, ""),
	}, ""))
	c := turns[0]
	if c.Edits["/w/a.go"] != 1 || c.Lines["/w/a.go"] != 0 || c.Errors != 1 {
		t.Errorf("edits %v, lines %v, errors %d", c.Edits, c.Lines, c.Errors)
	}
}

// A reply the person interrupted still used what it used, and Pi records it.
func TestAnInterruptedReplyIsStillCharged(t *testing.T) {
	turns := turnsOf(t, strings.Join([]string{
		user("a", "", "go"),
		line("message", "b", "a", `,"message":{"role":"assistant","content":[{"type":"text","text":"parti"}],"provider":"anthropic","model":"claude-opus-4-8","usage":{"input":50,"output":7,"cacheRead":0,"cacheWrite":0},"stopReason":"aborted","timestamp":2}`),
	}, ""))
	if turns[0].Tokens != (agent.Tokens{Input: 50, Output: 7}) {
		t.Errorf("tokens = %+v", turns[0].Tokens)
	}
}

// A session whose every request failed has turns and nothing to charge. That
// is no cost at all, not an unpriced one.
func TestFailedRequestsChargeNothing(t *testing.T) {
	turns := turnsOf(t, strings.Join([]string{
		user("a", "", "go"),
		line("message", "b", "a", `,"message":{"role":"assistant","content":[],"provider":"anthropic","model":"claude-opus-4-8","usage":{"input":0,"output":0,"cacheRead":0,"cacheWrite":0},"stopReason":"error","errorMessage":"429","timestamp":2}`),
	}, ""))
	if len(turns) != 1 || turns[0].Models != nil || turns[0].Errors != 0 {
		t.Errorf("turns = %+v", turns)
	}
}

// /fork can start the new session in another directory. The copy lands in a
// different project, and it is still a copy.
func TestAForkIntoAnotherProjectCountsOnlyItsOwnWork(t *testing.T) {
	root := t.TempDir()
	write := func(dir, name, body string) string {
		p := filepath.Join(root, dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	orig := write("--w-one--", "orig.jsonl", session("/w/one", "",
		user("a", "", "first"), reply("b", "a", "", 100)))
	write("--w-two--", "fork.jsonl", session("/w/two", orig,
		user("a", "", "first"), reply("b", "a", "", 100),
		user("c", "b", "carried on elsewhere"), reply("d", "c", "", 7)))

	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil || len(projects) != 2 {
		t.Fatalf("%v, %v", projects, err)
	}
	for _, p := range projects {
		sessions, err := src.Sessions(p)
		if err != nil || len(sessions) != 1 {
			t.Fatalf("%s: %v, %v", p.Name, sessions, err)
		}
		turns := sessions[0].Turns
		switch p.Name {
		case "one":
			if len(turns) != 1 || turns[0].Tokens.Input != 100 {
				t.Errorf("one = %+v", turns)
			}
		case "two":
			if len(turns) != 1 || turns[0].Text != "carried on elsewhere" || turns[0].Tokens.Input != 7 {
				t.Errorf("two counted the copy: %+v", turns)
			}
		}
	}
}

// A fork of a fork inherits from both, and counts neither's work again.
func TestAChainOfForksCountsEachPieceOnce(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "--w--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	a := filepath.Join(dir, "a.jsonl")
	b := filepath.Join(dir, "b.jsonl")
	c := filepath.Join(dir, "c.jsonl")
	one := user("1", "", "one") + reply("2", "1", "", 1)
	two := user("3", "2", "two") + reply("4", "3", "", 2)
	three := user("5", "4", "three") + reply("6", "5", "", 4)
	for p, body := range map[string]string{
		a: session("/w", "", one),
		b: session("/w", a, one, two),
		c: session("/w", b, one, two, three),
	} {
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := Source{Root: filepath.Dir(dir)}
	projects, err := src.Detect()
	if err != nil || len(projects) != 1 {
		t.Fatalf("%v, %v", projects, err)
	}
	sessions, err := src.Sessions(projects[0])
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	var texts []string
	for _, s := range sessions {
		for _, turn := range s.Turns {
			total += turn.Tokens.Input
			texts = append(texts, turn.Text)
		}
	}
	if total != 7 || len(texts) != 3 {
		t.Errorf("counted %d input tokens over %q, want 7 over one, two, three", total, texts)
	}
}

// A file with a header and nothing after it has no work in it.
func TestAnEmptySessionHasNoTurns(t *testing.T) {
	if turns := turnsOf(t, session("/w", "")); len(turns) != 0 {
		t.Errorf("turns = %+v", turns)
	}
}
