package pi

import (
	"encoding/json"
	"net"
	"net/url"
	"os"
	"strings"

	"github.com/nickelsec/bough/internal/price"
)

// unknownModel names model work the record gives no model for. It matches no
// rate, so work charged to it is shown as unpriced rather than left out.
const unknownModel = "unknown model"

// prefix is how bough's rate table, which comes from LiteLLM, spells each Pi
// provider's models.
//
// Pi can reach the same model through many providers, and they charge
// differently for it: Claude Opus costs one thing from Anthropic and another
// through a Bedrock region. So a model is priced under the provider it went
// through, and the key says which. Anthropic's and OpenAI's own models keep
// their bare names, the same names Claude Code and Codex record, so the same
// model reads the same whichever agent used it.
//
// openai-codex is a ChatGPT subscription rather than a per-token account, and
// it is priced at OpenAI's API rates on the same footing as Codex itself: what
// the work would have cost billed per token.
//
// The provider ids are Pi's, from packages/ai/src/types.ts. Every mapping here
// was checked against the models in Pi's own catalog, and a provider missing
// from this list is not guessed at: its models are named "provider/model",
// which no rate matches, so they are shown as unpriced.
var prefix = map[string]string{
	"anthropic":              "",
	"openai":                 "",
	"openai-codex":           "",
	"amazon-bedrock":         "",
	"google-vertex":          "",
	"google":                 "gemini/",
	"azure-openai-responses": "azure/",
	"openrouter":             "openrouter/",
	"vercel-ai-gateway":      "vercel_ai_gateway/",
	"xai":                    "xai/",
	"groq":                   "groq/",
	"cerebras":               "cerebras/",
	"mistral":                "mistral/",
	"deepseek":               "deepseek/",
	"zai":                    "zai/",
	"moonshotai":             "moonshot/",
	"minimax":                "minimax/",
	"fireworks":              "fireworks_ai/",
	"together":               "together_ai/",
	"baseten":                "baseten/",
	"cloudflare-workers-ai":  "cloudflare/",
	"meta":                   "meta/",
}

// Priced is the name a model's work is charged under: the model as the rate
// table spells it for the provider it went through.
func Priced(provider, model string) string {
	if p, ok := prefix[provider]; ok {
		return p + model
	}
	if provider == "" {
		return model
	}
	return provider + "/" + model
}

// builtinLocal is Pi's own llama.cpp provider, which only ever talks to a
// llama.cpp server and which Pi itself prices at nothing. See
// dist/extensions/llama/provider.js in the coding agent.
const builtinLocal = "llama.cpp"

// localServers are the names Pi's docs give local model servers when setting
// them up in models.json. They are only the fallback for when models.json
// cannot be read, history copied from another machine for instance: where it
// can be read, the endpoint it names decides.
var localServers = map[string]bool{
	"ollama":    true,
	"lmstudio":  true,
	"lm-studio": true,
	"vllm":      true,
	"sglang":    true,
}

// endpoints says, for each provider set up in Pi's models.json, whether its
// endpoint is on the person's own machine or network.
type endpoints map[string]bool

// name is what a piece of work is charged under.
//
// Work on a local model is marked so and priced at zero. Everything else goes
// by the rate table, and a model it does not hold is shown as unpriced.
//
// Local is decided carefully, because calling paid work free is the worst
// mistake a bill can make, and a local endpoint is not always a local model. A
// proxy on localhost that forwards to a hosted service looks exactly like
// Ollama from here. So it takes all of these: the endpoint is local, the
// provider is not one of Pi's hosted ones, the model is not one the rate table
// knows by name, and Pi itself charged nothing for the reply.
func (e endpoints) name(provider, model string, piCharged float64) string {
	if model == "" {
		return unknownModel
	}
	hosted := Priced(provider, model)
	if e.local(provider) && piCharged == 0 {
		if _, known := price.Of(model); !known {
			return provider + "/" + model + price.Local
		}
	}
	return hosted
}

func (e endpoints) local(provider string) bool {
	if provider == builtinLocal {
		return true
	}
	if _, hosted := prefix[provider]; hosted {
		return false
	}
	if l, ok := e[provider]; ok {
		return l
	}
	return localServers[strings.ToLower(provider)]
}

// readEndpoints reads which providers in Pi's models.json point at a local
// endpoint. A file that is missing or does not parse gives none, and the
// fallback names apply. See docs/models.md in the coding agent.
func readEndpoints(path string) endpoints {
	if path == "" {
		return nil
	}
	b, err := os.ReadFile(path) //#nosec G304 -- Pi's own models.json, read and never written.
	if err != nil {
		return nil
	}
	var cfg struct {
		Providers map[string]struct {
			BaseURL string `json:"baseUrl"`
		} `json:"providers"`
	}
	if json.Unmarshal(b, &cfg) != nil {
		return nil
	}
	out := endpoints{}
	for name, p := range cfg.Providers {
		if p.BaseURL != "" {
			out[name] = onThisNetwork(p.BaseURL)
		}
	}
	return out
}

// onThisNetwork reports whether a URL points at the person's own machine or a
// private network: localhost, a loopback, private or link-local address, or a
// ".local" name.
func onThisNetwork(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	switch {
	case host == "":
		return false
	case host == "localhost", strings.HasSuffix(host, ".localhost"),
		strings.HasSuffix(host, ".local"), host == "host.docker.internal":
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified())
}
