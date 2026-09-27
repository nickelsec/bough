package pi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/price"
)

// A models.json in the shape Pi's docs give it: Ollama and LM Studio on this
// machine, a vLLM box on the local network, a LiteLLM proxy on localhost that
// forwards to hosted models, and a hosted service reached through a custom
// provider. See docs/models.md in the coding agent.
const modelsJSON = `{
  "providers": {
    "ollama":   {"baseUrl": "http://localhost:11434/v1", "api": "openai-completions", "apiKey": "ollama", "models": [{"id": "qwen2.5-coder:7b"}]},
    "studio":   {"baseUrl": "http://127.0.0.1:1234/v1", "api": "openai-completions"},
    "gpu-box":  {"baseUrl": "http://192.168.1.40:8000/v1", "api": "openai-completions"},
    "litellm":  {"baseUrl": "http://localhost:4000", "api": "openai-completions"},
    "acme":     {"baseUrl": "https://api.acme.example/v1", "api": "openai-completions"},
    "openai":   {"baseUrl": "http://localhost:9999/v1"}
  }
}`

func endpointsFor(t *testing.T) endpoints {
	t.Helper()
	p := filepath.Join(t.TempDir(), "models.json")
	if err := os.WriteFile(p, []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	return readEndpoints(p)
}

func TestWhatCountsAsLocal(t *testing.T) {
	e := endpointsFor(t)
	cases := []struct {
		provider, model string
		charged         float64
		want            string
	}{
		// On this machine or network: priced at nothing, and saying why.
		{"ollama", "qwen2.5-coder:7b", 0, "ollama/qwen2.5-coder:7b" + price.Local},
		{"studio", "mistral-7b", 0, "studio/mistral-7b" + price.Local},
		{"gpu-box", "llama-4-70b", 0, "gpu-box/llama-4-70b" + price.Local},
		// Pi's own llama.cpp provider, with or without a models.json.
		{"llama.cpp", "gemma-4-12b", 0, "llama.cpp/gemma-4-12b" + price.Local},

		// A local proxy naming a hosted model is forwarding to a paid service,
		// so it is not free. Its provider has no rate, so it is unpriced.
		{"litellm", "claude-opus-4-8", 0, "litellm/claude-opus-4-8"},
		// A hosted service through a custom provider: unpriced, not free.
		{"acme", "acme-large", 0, "acme/acme-large"},
		// A built-in hosted provider pointed at a local proxy is still that
		// provider's model at that provider's rate.
		{"openai", "gpt-5.6-sol", 0, "gpt-5.6-sol"},
		// Pi charged for it, so somebody set a price: it is not local work.
		{"ollama", "qwen2.5-coder:7b", 0.01, "ollama/qwen2.5-coder:7b"},
	}
	for _, c := range cases {
		if got := e.name(c.provider, c.model, c.charged); got != c.want {
			t.Errorf("name(%q, %q, %v) = %q, want %q", c.provider, c.model, c.charged, got, c.want)
		}
	}
}

// Without a models.json, history read on another machine for instance, the
// names Pi's docs use for local servers still count, and nothing else does.
func TestLocalWithoutModelsJSON(t *testing.T) {
	var e endpoints
	if got := e.name("ollama", "qwen2.5-coder:7b", 0); got != "ollama/qwen2.5-coder:7b"+price.Local {
		t.Errorf("ollama without models.json = %q", got)
	}
	if got := e.name("llama.cpp", "gemma", 0); got != "llama.cpp/gemma"+price.Local {
		t.Errorf("llama.cpp without models.json = %q", got)
	}
	if got := e.name("mybox", "qwen", 0); got != "mybox/qwen" {
		t.Errorf("an unknown provider was taken as local: %q", got)
	}
	if got := readEndpoints(filepath.Join(t.TempDir(), "missing.json")); got != nil {
		t.Errorf("a missing file gave %v", got)
	}
}

func TestOnThisNetwork(t *testing.T) {
	for raw, want := range map[string]bool{
		"http://localhost:11434/v1":         true,
		"http://127.0.0.1:8080":             true,
		"http://[::1]:8080":                 true,
		"http://0.0.0.0:8000":               true,
		"http://10.0.0.5/v1":                true,
		"http://172.20.1.1/v1":              true,
		"http://192.168.1.40:8000":          true,
		"http://studio.local:1234":          true,
		"http://host.docker.internal:11434": true,
		"https://api.openai.com/v1":         false,
		"https://8.8.8.8/v1":                false,
		"http://172.32.0.1/v1":              false,
		"not a url at all":                  false,
		"":                                  false,
	} {
		if got := onThisNetwork(raw); got != want {
			t.Errorf("onThisNetwork(%q) = %v, want %v", raw, got, want)
		}
	}
}

// End to end: a project whose sessions sit in an agent directory beside its
// models.json prices local work at nothing and names a model it cannot price.
func TestSessionsPriceLocalWorkAtNothing(t *testing.T) {
	agentDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(agentDir, "models.json"), []byte(modelsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(agentDir, "sessions")
	dir := filepath.Join(root, "--w--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `{"type":"session","version":3,"id":"s","timestamp":"2026-09-20T10:00:00Z","cwd":"/w"}` + "\n" +
		line("message", "a", "", `,"message":{"role":"user","content":"hi","timestamp":1}`) +
		line("message", "b", "a", `,"message":{"role":"assistant","content":[],"provider":"ollama","model":"qwen2.5-coder:7b","usage":{"input":900,"output":80,"cacheRead":0,"cacheWrite":0,"cost":{"total":0}},"stopReason":"stop","timestamp":2}`) +
		line("message", "c", "b", `,"message":{"role":"user","content":"and again","timestamp":3}`) +
		line("message", "d", "c", `,"message":{"role":"assistant","content":[],"provider":"acme","model":"acme-large","usage":{"input":100,"output":10,"cacheRead":0,"cacheWrite":0,"cost":{"total":0.002}},"stopReason":"stop","timestamp":4}`)
	if err := os.WriteFile(filepath.Join(dir, "s.jsonl"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	src := Source{Root: root}
	projects, err := src.Detect()
	if err != nil || len(projects) != 1 {
		t.Fatalf("%v, %v", projects, err)
	}
	sessions, err := src.Sessions(projects[0])
	if err != nil || len(sessions) != 1 || len(sessions[0].Turns) != 2 {
		t.Fatalf("sessions = %+v, %v", sessions, err)
	}

	local := sessions[0].Turns[0].Models
	if c := price.Spend(local); !c.Priced || c.Dollars != 0 {
		t.Errorf("local turn = %+v, want priced at nothing", c)
	}
	hosted := sessions[0].Turns[1].Models
	if c := price.Spend(hosted); c.Priced || len(c.Unpriced) != 1 || c.Unpriced[0] != "acme/acme-large" {
		t.Errorf("unknown hosted turn = %+v, want unpriced and named", c)
	}

	all := map[string]agent.Tokens{}
	for _, turn := range sessions[0].Turns {
		agent.Merge(&all, turn.Models)
	}
	if c := price.Spend(all); c.Priced {
		t.Errorf("the project total was priced with an unknown model in it: %+v", c)
	}
}
