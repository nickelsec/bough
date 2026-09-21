// Command gen writes the built-in rate table from LiteLLM's published prices.
//
// Run it with "go generate ./internal/price". It is the only thing in the
// project that reaches the network, and it runs when someone decides to update
// the rates rather than when anyone reads their history.
//
// The table is trimmed hard on the way in. LiteLLM's file is about 2.8MB
// covering four thousand models on every provider; what bough can ever see is
// the chat models of the two agents it reads, which is a couple of hundred
// entries and around 20KB of Go.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// source is LiteLLM's published table of model prices.
const source = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"

// entry is the part of a LiteLLM record this cares about. Everything else in
// there describes what a model can do rather than what it charges.
type entry struct {
	Provider   string  `json:"litellm_provider"`
	Mode       string  `json:"mode"`
	Input      float64 `json:"input_cost_per_token"`
	Output     float64 `json:"output_cost_per_token"`
	CacheRead  float64 `json:"cache_read_input_token_cost"`
	CacheWrite float64 `json:"cache_creation_input_token_cost"`

	// Storing context for an hour rather than five minutes. Claude Code uses
	// the hourly cache for nearly everything it writes, so leaving this out
	// under-charged the dearest line on the bill.
	CacheWriteHour float64 `json:"cache_creation_input_token_cost_above_1hr"`

	// The above-200k twins. Their values are not kept, only whether they are
	// there: bough cannot tell which side of the line a request fell on, so a
	// model that charges by context size is marked unpriceable rather than
	// charged at either rate.
	InputAbove      *float64 `json:"input_cost_per_token_above_200k_tokens"`
	OutputAbove     *float64 `json:"output_cost_per_token_above_200k_tokens"`
	CacheReadAbove  *float64 `json:"cache_read_input_token_cost_above_200k_tokens"`
	CacheWriteAbove *float64 `json:"cache_creation_input_token_cost_above_200k_tokens"`
}

// wanted keeps the providers whose models bough can actually meet. Claude Code
// reports Anthropic models and Codex reports OpenAI ones.
var wanted = map[string]bool{"anthropic": true, "openai": true}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	raw, err := fetch()
	if err != nil {
		return err
	}

	var all map[string]json.RawMessage
	if err := json.Unmarshal(raw, &all); err != nil {
		return fmt.Errorf("parsing the table: %w", err)
	}

	names := make([]string, 0, len(all))
	kept := map[string]entry{}
	for name, blob := range all {
		var e entry
		// A few records are not objects at all. They are not models.
		if json.Unmarshal(blob, &e) != nil {
			continue
		}
		if !wanted[e.Provider] || e.Mode != "chat" {
			continue
		}
		// A model that charges nothing for input and output is not a model
		// bough can price; it is an entry with the prices left out.
		if e.Input == 0 && e.Output == 0 {
			continue
		}
		kept[name] = e
		names = append(names, name)
	}
	if len(names) == 0 {
		return errors.New("no models matched, the table's shape has probably changed")
	}
	sort.Strings(names)

	var b strings.Builder
	fmt.Fprint(&b, header)
	fmt.Fprintf(&b, "// Date is when these were read from LiteLLM.\n")
	fmt.Fprintf(&b, "var Date = time.Date(%d, %d, %d, 0, 0, 0, 0, time.UTC)\n\n",
		time.Now().UTC().Year(), int(time.Now().UTC().Month()), time.Now().UTC().Day())
	fmt.Fprintf(&b, "// rates is what each model charges per token.\nvar rates = map[string]Rate{\n")
	for _, name := range names {
		e := kept[name]
		tiered := e.InputAbove != nil || e.OutputAbove != nil ||
			e.CacheReadAbove != nil || e.CacheWriteAbove != nil
		fmt.Fprintf(&b, "\t%q: {Input: %s, Output: %s, CacheRead: %s, CacheWrite: %s",
			name, num(e.Input), num(e.Output), num(e.CacheRead), num(e.CacheWrite))
		if e.CacheWriteHour != 0 {
			fmt.Fprintf(&b, ", CacheWriteHour: %s", num(e.CacheWriteHour))
		}
		if tiered {
			fmt.Fprint(&b, ", Tiered: true")
		}
		fmt.Fprint(&b, "},\n")
	}
	fmt.Fprint(&b, "}\n")

	// Formatted here rather than left for whoever runs it to notice, so a
	// regenerated table never fails the build on layout alone.
	src, err := format.Source([]byte(b.String()))
	if err != nil {
		return fmt.Errorf("the generated table does not parse: %w", err)
	}
	if err := os.WriteFile("rates.go", src, 0o600); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gen: wrote %d models\n", len(names))
	return nil
}

func fetch() ([]byte, error) {
	c := &http.Client{Timeout: 60 * time.Second}
	resp, err := c.Get(source)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching the table: %s", resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// num writes a rate the way Go source wants it, without losing a digit.
func num(f float64) string {
	if f == 0 {
		return "0"
	}
	return fmt.Sprintf("%g", f)
}

const header = `// Code generated by internal/price/gen. DO NOT EDIT.
//
// Rates published by LiteLLM. Refresh with
// "go generate ./internal/price".

package price

import "time"

`
