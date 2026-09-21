// Package price says what a model charges, and works out what a stretch of
// work would have cost at those rates.
//
// The rates come from LiteLLM's published table, which is the table the wider
// ecosystem prices against, so a figure here can be checked against the same
// source anyone else would use. They are built into the binary rather than
// fetched, because reading history is meant to work with the network
// unplugged. That makes them a snapshot: Date says when it was taken, and
// anything shown to a reader is expected to say so.
//
// The rule everywhere below is that a wrong number is worse than no number. A
// model with no entry, or one charged in a way this cannot express, returns
// no price rather than a guess.
package price

//go:generate go run ./gen

import (
	"strings"
	"time"

	"github.com/nickelsec/bough/internal/agent"
)

// Rate is what one model charges, per token.
//
// Fractions of a cent per token, so the figures are tiny: 5e-06 is five dollars
// per million. They are kept as the table states them rather than scaled, so
// they can be read straight across against the published rates.
type Rate struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64

	// CacheWriteHour is storing context for an hour instead of five minutes,
	// which costs about 60% more. Zero where a model does not offer it, and
	// then the ordinary rate stands.
	CacheWriteHour float64

	// Tiered says this model charges different rates past a context size.
	//
	// Working out which side of that line a request fell on needs the size of
	// each request, and bough keeps only per-turn sums, so a tiered model
	// cannot be priced honestly here. It is recorded rather than ignored so
	// pricing can refuse, instead of quietly charging the cheaper rate and
	// under-reporting exactly the longest sessions.
	Tiered bool
}

// Of finds the rate for a model, reporting whether one is known.
//
// An exact match first, which is what every model in the corpus this was
// checked against needed. Failing that, a trailing release date is dropped:
// Claude Code has historically written names like
// "claude-sonnet-4-5-20250929" for a model the table lists as
// "claude-sonnet-4-5". Nothing else is attempted. Guessing from a prefix would
// eventually match a cheaper or dearer sibling and bill the work at it.
func Of(model string) (Rate, bool) {
	if model == "" {
		return Rate{}, false
	}
	if r, ok := rates[model]; ok {
		return r, true
	}
	if base, cut := withoutDate(model); cut {
		if r, ok := rates[base]; ok {
			return r, true
		}
	}
	return Rate{}, false
}

// withoutDate drops a trailing "-20250929" style release date.
func withoutDate(model string) (string, bool) {
	const stamp = 8 // 20250929
	if len(model) < stamp+1 {
		return model, false
	}
	cut := len(model) - stamp
	if model[cut-1] != '-' {
		return model, false
	}
	for i := cut; i < len(model); i++ {
		if model[i] < '0' || model[i] > '9' {
			return model, false
		}
	}
	return model[:cut-1], true
}

// Cost is what a stretch of work would have cost, in dollars.
//
// Priced at published API rates. A flat-rate subscription pays none of this,
// which is why nothing here calls the figure "spent": it is what the same work
// would have cost had it been billed per token.
type Cost struct {
	// Dollars is the figure. Meaningless unless Priced is true.
	Dollars float64

	// Priced says every model in the work had a known rate. When false the
	// figure covers only part of the work and must not be shown as a total.
	Priced bool

	// Unpriced names the models that had no usable rate, sorted, so the
	// reason can be given rather than just the refusal.
	Unpriced []string
}

// Spend works out what a per-model set of token counts would have cost.
//
// Every model must price for the answer to count. Adding up the ones that do
// and calling it a total is how a bill silently loses its largest line.
func Spend(models map[string]agent.Tokens) Cost {
	var out Cost
	if len(models) == 0 {
		return out
	}
	out.Priced = true
	for model, t := range models {
		r, ok := Of(model)
		if !ok || r.Tiered {
			out.Priced = false
			out.Unpriced = append(out.Unpriced, model)
			continue
		}
		out.Dollars += At(r, t)
	}
	sortStrings(out.Unpriced)
	return out
}

// At prices one set of token counts at one rate.
//
// The four are multiplied separately because they are charged separately, and
// by very different amounts: re-reading the cache costs about a tenth of fresh
// input and writing it about a quarter more. A blended rate would be wrong by
// more than the figure itself on most work, since cache reads are around 99%
// of every count.
func At(r Rate, t agent.Tokens) float64 {
	// The hourly cache writes are part of CacheWrite, not extra to it, so the
	// cheaper rate is charged on what is left after taking them out.
	hour := t.CacheWriteHour
	if hour > t.CacheWrite {
		hour = t.CacheWrite
	}
	hourly := r.CacheWriteHour
	if hourly == 0 {
		hourly = r.CacheWrite
	}
	return float64(t.Input)*r.Input +
		float64(t.Output)*r.Output +
		float64(t.CacheRead)*r.CacheRead +
		float64(t.CacheWrite-hour)*r.CacheWrite +
		float64(hour)*hourly
}

// Taken is when the built-in rates were read from LiteLLM.
func Taken() time.Time { return Date }

// sortStrings is sort.Strings without the import, which is not worth pulling
// in for a list that is almost always empty and never long.
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && strings.Compare(s[j-1], s[j]) > 0; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
