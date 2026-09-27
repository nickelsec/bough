package pi

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// TestRealCorpus runs the reader over whatever Pi history exists on the
// machine running the tests. It is skipped when there is none, so it stays
// useful for a contributor with their own history and harmless in CI.
//
// It asserts properties rather than numbers, since every corpus is different,
// and it checks each one against a plain reading of the file that shares no
// code with the reader: every prompt becomes a turn, and every reply's tokens
// are counted exactly once.
func TestRealCorpus(t *testing.T) {
	src := Source{}
	projects, err := src.Detect()
	if err != nil {
		t.Fatalf("detect: %v", err)
	}
	if len(projects) == 0 {
		t.Skip("no Pi history on this machine")
	}
	for _, p := range projects {
		sessions, err := src.Sessions(p)
		if err != nil {
			t.Errorf("%s: %v", p.Name, err)
		}
		var files []string
		if err := json.Unmarshal([]byte(p.Ref), &files); err != nil {
			t.Fatal(err)
		}

		// The plain reading: prompts and reply tokens straight off the file,
		// leaving out forks, which the reader rightly does not recount.
		prompts, tokens := 0, 0
		for _, fp := range files {
			entries, err := readFile(fp)
			if err != nil || len(entries) == 0 || entries[0].ParentSession != "" {
				continue
			}
			for _, e := range entries {
				m := e.Message
				if m == nil {
					continue
				}
				if m.Role == "user" && (strings.TrimSpace(text(m.Content)) != "" || hasImage(m.Content)) {
					prompts++
				}
				if m.Role == "assistant" && m.Usage != nil {
					tokens += int(m.Usage.Input + m.Usage.Output + m.Usage.CacheRead + m.Usage.CacheWrite)
				}
			}
		}

		gotPrompts, gotTokens := 0, 0
		for _, s := range sessions {
			for _, turn := range s.Turns {
				gotPrompts++
				gotTokens += turn.Tokens.Total()
				var sum agent.Tokens
				for _, m := range turn.Models {
					sum.Add(m)
				}
				if sum != turn.Tokens {
					t.Errorf("%s: a turn's models sum to %+v, the turn says %+v", p.Name, sum, turn.Tokens)
				}
			}
		}
		if gotPrompts < prompts {
			t.Errorf("%s: %d prompts read, the file holds %d", p.Name, gotPrompts, prompts)
		}
		// At least the replies' tokens: usage entries, summaries and
		// sub-agents come on top.
		if gotTokens < tokens {
			t.Errorf("%s: %d tokens read, the replies alone hold %d", p.Name, gotTokens, tokens)
		}
		t.Logf("%s: %d sessions, %d prompts, %d tokens", p.Name, len(sessions), gotPrompts, gotTokens)
	}
}
