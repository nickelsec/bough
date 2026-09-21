package graph

// This file is kept apart from graph.go, which holds what is true about the
// work and nothing that depends on anything outside it. A bill is not in that
// category: the same history costs different amounts as published rates
// change, so the figure is worked out on request rather than written into the
// record.

import (
	"github.com/nickelsec/bough/internal/agent"
	"github.com/nickelsec/bough/internal/price"
)

// Spend is what this work would have cost at published API rates, and whether
// every model in it had a rate to price with.
//
// Worked out here rather than written into the graph, because it is not a fact
// about the work: it depends on a price table that changes under the same
// history. Anyone drawing this is expected to say what the rates were taken
// from and when.
//
// A flat-rate subscription pays none of this. It is what the same work would
// have cost had it been billed per token, which is the only figure the record
// supports: nothing in a transcript says what anyone was actually charged.
func (s Stats) Spend() price.Cost {
	if len(s.Models) == 0 {
		return price.Cost{}
	}
	by := make(map[string]agent.Tokens, len(s.Models))
	for model, t := range s.Models {
		by[model] = agent.Tokens{
			Input:          t.Input,
			Output:         t.Output,
			CacheRead:      t.CacheRead,
			CacheWrite:     t.CacheWrite,
			CacheWriteHour: t.CacheWriteHour,
		}
	}
	return price.Spend(by)
}
