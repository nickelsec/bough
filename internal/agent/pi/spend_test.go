package pi

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/price"
)

// piRecorded adds up what Pi itself worked out a session cost, from the cost
// it writes beside every piece of usage.
//
// That is every usage Pi counts in a session's total: replies, usage entries,
// compaction and branch summaries, and tools' nested work. On top of those it
// adds the sub-agents Pi leaves out and bough counts: each sub-agent's own
// replies where the result carries them, or its summed usage where it does not.
func piRecorded(t *testing.T, entries []*Entry) float64 {
	t.Helper()
	total := 0.0
	add := func(u *Usage) {
		if u != nil {
			total += u.Cost.Total
		}
	}
	for _, e := range entries {
		add(e.Usage)
		if e.Message == nil {
			continue
		}
		add(e.Message.Usage)
		if e.Message.ToolName != "subagent" {
			continue
		}
		var d struct {
			Results []struct {
				Messages []*Message `json:"messages"`
				Usage    *Usage     `json:"usage"`
			} `json:"results"`
		}
		if err := json.Unmarshal(e.Message.Details, &d); err != nil {
			t.Fatal(err)
		}
		for _, r := range d.Results {
			if len(r.Messages) == 0 {
				add(r.Usage)
				continue
			}
			for _, m := range r.Messages {
				if m.Role == "assistant" {
					add(m.Usage)
				}
			}
		}
	}
	return total
}

func boughCost(t *testing.T, turns []agent.Turn) price.Cost {
	t.Helper()
	all := map[string]agent.Tokens{}
	for _, turn := range turns {
		agent.Merge(&all, turn.Models)
	}
	return price.Spend(all)
}

// bough's figure has to land on Pi's own for the session this was checked
// against: a real one, across a ChatGPT plan's models and a free OpenRouter
// one. The two price from different tables, LiteLLM's and Pi's, so agreement
// says the counts, the model names and the provider mapping are all right.
func TestCostMatchesPiOnARealSession(t *testing.T) {
	entries := readFixture(t, "real.jsonl")
	turns := ExtractTurns(entries[1:], entries[0].Cwd)
	got := boughCost(t, turns)
	if !got.Priced {
		t.Fatalf("refused to price: %v", got.Unpriced)
	}
	want := piRecorded(t, entries)
	if math.Abs(got.Dollars-want) > 1e-9 {
		t.Errorf("bough says $%.8f, Pi recorded $%.8f", got.Dollars, want)
	}
}

// The same on the fixture Pi's own SessionManager wrote, which adds an hourly
// cache, cache warming, a compaction, a branch summary, a tool's nested work
// and two sub-agents.
//
// The clone is left out, and knowingly. A summary names no model, so it is
// charged to the one in use, which is right in a file written as the work
// happened. A clone keeps only the path to where the person ended up, and on
// that path the branch summary follows the model the abandoned branch started
// with rather than the one in use when the person jumped. Read on its own, a
// clone prices its summary at the wrong model. Read beside its original, which
// is the usual case, those entries are the original's and never reach it.
func TestCostMatchesPiOnEveryKindOfUsage(t *testing.T) {
	for _, name := range []string{"main", "fork"} {
		entries := readFixture(t, "sessions/"+name+".jsonl")
		turns := ExtractTurns(entries[1:], entries[0].Cwd)
		got := boughCost(t, turns)
		if !got.Priced {
			t.Fatalf("%s: refused to price: %v", name, got.Unpriced)
		}
		want := piRecorded(t, entries)
		// Relative, because the fixture's figures are small and the two
		// tables write their rates to different precision.
		if math.Abs(got.Dollars-want) > 1e-9+want*1e-6 {
			t.Errorf("%s: bough says $%.8f, Pi recorded $%.8f", name, got.Dollars, want)
		}
	}
}

// Every provider that maps into the table has to land on real keys. Checked
// against models Pi's own catalog lists, one per provider.
func TestProvidersMapOntoTheTable(t *testing.T) {
	cases := map[[2]string]string{
		{"anthropic", "claude-opus-4-8"}:                         "claude-opus-4-8",
		{"openai", "gpt-5.6-sol"}:                                "gpt-5.6-sol",
		{"openai-codex", "gpt-5.6-luna"}:                         "gpt-5.6-luna",
		{"openrouter", "nvidia/nemotron-3-ultra-550b-a55b:free"}: "openrouter/nvidia/nemotron-3-ultra-550b-a55b:free",
		{"google", "gemini-2.5-pro"}:                             "gemini/gemini-2.5-pro",
		{"google-vertex", "gemini-2.5-pro"}:                      "gemini-2.5-pro",
		{"amazon-bedrock", "anthropic.claude-opus-4-8"}:          "anthropic.claude-opus-4-8",
		{"azure-openai-responses", "gpt-5.6-sol"}:                "azure/gpt-5.6-sol",
		{"groq", "openai/gpt-oss-120b"}:                          "groq/openai/gpt-oss-120b",
		{"together", "deepseek-ai/DeepSeek-V4-Flash-0731"}:       "",
		{"cloudflare-workers-ai", "@cf/openai/gpt-oss-120b"}:     "cloudflare/@cf/openai/gpt-oss-120b",
		{"baseten", "zai-org/GLM-5.2"}:                           "baseten/zai-org/GLM-5.2",
		{"meta", "muse-spark-1.3"}:                               "meta/muse-spark-1.3",
	}
	for in, want := range cases {
		got := Priced(in[0], in[1])
		if want == "" {
			continue // mapped, but not a model the table happens to list
		}
		if got != want {
			t.Errorf("Priced(%q, %q) = %q, want %q", in[0], in[1], got, want)
		}
		if _, ok := price.Of(got); !ok {
			t.Errorf("%s/%s maps to %q, which the table does not hold", in[0], in[1], got)
		}
	}
}

// A provider nobody mapped is not guessed at. Its models are named in full and
// shown as unpriced.
func TestAnUnknownProviderIsNotGuessed(t *testing.T) {
	got := Priced("opencode", "claude-fable-5")
	if got != "opencode/claude-fable-5" {
		t.Errorf("got %q", got)
	}
	if _, ok := price.Of(got); ok {
		t.Error("an unmapped provider's model was priced")
	}
	if Priced("", "gpt-5.6-sol") != "gpt-5.6-sol" {
		t.Error("a model with no provider lost its name")
	}
}

// The real fixture was cut from a real session, which holds a home directory,
// a person's name and possibly a pasted credential. This checks the scrubbing
// held, so a future regeneration cannot quietly publish any of it. Home
// directories are allowed only as the placeholder the scrubbing puts in.
func TestFixtureCarriesNoRealData(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "real.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	s := strings.ToLower(string(data))
	for _, leak := range []string{"sk-", "ghp_", "akia", "-----begin", "@gmail", "/home/", "encrypted_content", "thinkingsignature"} {
		if strings.Contains(s, leak) {
			t.Errorf("fixture contains %q, it needs scrubbing before it can ship", leak)
		}
	}
	found := homeDir.FindAllStringSubmatch(s, -1)
	for _, m := range found {
		if m[1] != "someone" {
			t.Errorf("fixture names a home directory %q", m[0])
		}
	}

	// The check has to be able to fail. The session ran from a home
	// directory, so a pattern that finds none has stopped looking.
	if len(found) == 0 {
		t.Error("found no home directory at all, so the pattern is not matching")
	}
	for _, bad := range []string{`c:\\users\\bob\\x`, `c:\users\bob`, `/users/bob/x`} {
		if m := homeDir.FindStringSubmatch(bad); m == nil || m[1] != "bob" {
			t.Errorf("the pattern misses %q", bad)
		}
	}
}

// homeDir finds a home directory in the file, however its separator was
// escaped: "users/x", "users\x", or "users\\x" as JSON writes it.
var homeDir = regexp.MustCompile(`users(?:\\|/)+([^\\/"\s]+)`)
