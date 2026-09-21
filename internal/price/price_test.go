package price

import (
	"math"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// The figure has to land where an independent reckoning of the same history
// lands.
//
// These are the token counts from one real session and the bill worked out
// from them by hand, against the published rates, outside this package. It is
// worth pinning because a change to how the counts are multiplied would still
// look entirely plausible on screen: the failure mode of a price is not a
// crash, it is a number nobody questions.
//
// Within a quarter of a percent rather than to the cent, and the slack is
// deliberate. The reference figure comes from one flat cache-write total,
// which cannot say how it divides between the five minute and hourly rates;
// this reconstructs that split from the transcripts instead. The two ways of
// splitting it differ by 1.4% on a line that is 8% of the bill. Anything
// outside a quarter of a percent overall means the method diverged rather
// than a few records being counted differently.
func TestPricesMatchAnIndependentReckoning(t *testing.T) {
	// Claude Code writes its cache with the hourly time to live: across this
	// project's history the five minute figure is zero on every record.
	spend := map[string]agent.Tokens{
		"claude-opus-5": {
			Input:          10828,
			Output:         2530223,
			CacheRead:      1722526059,
			CacheWrite:     13726041,
			CacheWriteHour: 13726041,
		},
	}
	const reference = 1059.9328682499995

	got := Spend(spend)
	if !got.Priced {
		t.Fatalf("refused to price a known model: %v", got.Unpriced)
	}
	off := math.Abs(got.Dollars-reference) / reference
	if off > 0.0025 {
		t.Errorf("priced at %.4f against a reference of %.4f, which is %.2f%% apart",
			got.Dollars, reference, off*100)
	}
}

// Charging every cache write at the five minute rate is the mistake this
// guards, and it is the one that shipped for a day.
//
// Claude Code stores context for an hour, which costs 60% more, and the flat
// cache_creation_input_tokens field says nothing about which. Reading that
// field alone and pricing it cheap put this project's bill $49 under the
// published rates on a thousand dollar total, which is small enough to look
// like rounding.
func TestHourlyCacheIsDearerThanFiveMinute(t *testing.T) {
	r, ok := Of("claude-opus-5")
	if !ok {
		t.Skip("claude-opus-5 is not in the table")
	}
	if r.CacheWriteHour <= r.CacheWrite {
		t.Fatalf("hourly rate %v is not dearer than the five minute rate %v",
			r.CacheWriteHour, r.CacheWrite)
	}
	const n = 1_000_000
	cheap := At(r, agent.Tokens{CacheWrite: n})
	dear := At(r, agent.Tokens{CacheWrite: n, CacheWriteHour: n})
	if dear <= cheap {
		t.Errorf("hourly cache priced at %v, five minute at %v", dear, cheap)
	}
	if want := float64(n) * r.CacheWriteHour; math.Abs(dear-want) > 1e-9 {
		t.Errorf("all-hourly priced %v, want %v", dear, want)
	}
}

// The hourly count is a slice of the cache write figure, not a fifth count.
//
// A record whose halves disagree with their own total must not let the dearer
// rate be charged on more tokens than were written.
func TestHourlyCacheNeverExceedsWhatWasWritten(t *testing.T) {
	r := Rate{CacheWrite: 1e-6, CacheWriteHour: 2e-6}
	// Claims more hourly tokens than were written at all.
	got := At(r, agent.Tokens{CacheWrite: 100, CacheWriteHour: 500})
	if want := 100 * 2e-6; math.Abs(got-want) > 1e-12 {
		t.Errorf("got %v, want %v: charged for more than was written", got, want)
	}
}

// A model with no separate hourly rate charges its ordinary one.
func TestNoHourlyRateFallsBackToTheOrdinaryOne(t *testing.T) {
	r := Rate{CacheWrite: 3e-6}
	got := At(r, agent.Tokens{CacheWrite: 1000, CacheWriteHour: 1000})
	if want := 1000 * 3e-6; math.Abs(got-want) > 1e-12 {
		t.Errorf("got %v, want %v", got, want)
	}
}

// Each of the four is charged at its own rate.
//
// Cache reads are around 99% of every count and are charged about a tenth of
// fresh input, so a blended rate would be wrong by more than the figure. A
// thousand tokens through each field, priced separately, catches any field
// that was dropped or paired with the wrong rate.
func TestEachCountIsChargedAtItsOwnRate(t *testing.T) {
	r := Rate{Input: 1e-6, Output: 2e-6, CacheRead: 4e-6, CacheWrite: 8e-6}
	got := At(r, agent.Tokens{Input: 1000, Output: 1000, CacheRead: 1000, CacheWrite: 1000})
	want := 1000 * (1e-6 + 2e-6 + 4e-6 + 8e-6)
	if math.Abs(got-want) > 1e-12 {
		t.Errorf("got %v, want %v", got, want)
	}
}

// A model nothing knows the price of must not be quietly charged nothing.
func TestUnknownModelRefusesToPrice(t *testing.T) {
	got := Spend(map[string]agent.Tokens{"a-model-nobody-published": {Output: 1000}})
	if got.Priced {
		t.Error("priced a model with no published rate")
	}
	if len(got.Unpriced) != 1 || got.Unpriced[0] != "a-model-nobody-published" {
		t.Errorf("Unpriced = %v, want the one model named", got.Unpriced)
	}
}

// One unknown model among known ones spoils the total rather than being
// skipped. A bill missing its largest line still looks like a bill.
func TestOneUnknownModelSpoilsTheTotal(t *testing.T) {
	got := Spend(map[string]agent.Tokens{
		"claude-opus-5": {Output: 1000},
		"who-knows":     {Output: 1000},
	})
	if got.Priced {
		t.Error("called a partial sum a total")
	}
	if len(got.Unpriced) != 1 || got.Unpriced[0] != "who-knows" {
		t.Errorf("Unpriced = %v, want only the unknown one", got.Unpriced)
	}
}

// A model charged by context size cannot be priced from per-turn sums.
//
// The rate depends on how big each single request was, and bough keeps only
// what a whole turn came to. Charging the under-200k rate would look right and
// under-report exactly the longest sessions, which are the ones worth knowing
// about.
func TestTieredModelRefusesToPrice(t *testing.T) {
	var tiered string
	for name, r := range rates {
		if r.Tiered {
			tiered = name
			break
		}
	}
	if tiered == "" {
		t.Skip("no tiered model in the table")
	}
	got := Spend(map[string]agent.Tokens{tiered: {Output: 1000}})
	if got.Priced {
		t.Errorf("priced %s, which charges by context size", tiered)
	}
}

// Claude Code writes dated model names for models the table lists undated.
func TestADatedNameFindsItsModel(t *testing.T) {
	want, ok := rates["claude-opus-5"]
	if !ok {
		t.Skip("claude-opus-5 is not in the table")
	}
	got, ok := Of("claude-opus-5-20250929")
	if !ok {
		t.Fatal("a dated name found no rate")
	}
	if got != want {
		t.Errorf("got %+v, want the undated model's rate %+v", got, want)
	}
}

// Dropping a date must not turn one model into a different one. Only a real
// eight digit stamp counts, and what is left has to be a model in its own
// right.
func TestOnlyARealDateIsDropped(t *testing.T) {
	for _, name := range []string{"gpt-4o-mini", "claude-opus-5", "not-a-model-1234", "x-2025092"} {
		if base, cut := withoutDate(name); cut && base == name {
			t.Errorf("%s: cut but unchanged", name)
		}
	}
	if base, cut := withoutDate("claude-sonnet-4-5-20250929"); !cut || base != "claude-sonnet-4-5" {
		t.Errorf("got %q cut=%v, want claude-sonnet-4-5", base, cut)
	}
	// Eight digits that are not preceded by a dash are part of the name.
	if _, cut := withoutDate("model20250929"); cut {
		t.Error("cut a name with no dash before the digits")
	}
}

// Nothing at all costs nothing, and says so rather than refusing.
func TestNoModelsIsNotAFailure(t *testing.T) {
	got := Spend(nil)
	if got.Dollars != 0 || len(got.Unpriced) != 0 {
		t.Errorf("got %+v, want an empty cost", got)
	}
}

// The table has to hold the models bough actually meets, or every figure it
// produces is a refusal.
func TestTheTableHoldsTheModelsWeSee(t *testing.T) {
	for _, name := range []string{"claude-opus-5", "gpt-6-astra"} {
		if _, ok := Of(name); !ok {
			t.Errorf("no rate for %s, which real history carries", name)
		}
	}
}

// Every rate has to be positive and in a plausible range. A generator that
// silently wrote zeroes would make every figure nought without failing.
func TestNoRateIsAbsurd(t *testing.T) {
	if len(rates) < 50 {
		t.Fatalf("only %d models in the table, the generator probably filtered wrong", len(rates))
	}
	for name, r := range rates {
		if r.Input <= 0 || r.Output <= 0 {
			t.Errorf("%s: input %v output %v, want both above zero", name, r.Input, r.Output)
		}
		// A dollar per token would be five orders of magnitude off.
		if r.Input > 1e-2 || r.Output > 1e-2 {
			t.Errorf("%s: rates look like the wrong unit: %+v", name, r)
		}
		if r.CacheRead < 0 || r.CacheWrite < 0 {
			t.Errorf("%s: negative cache rate: %+v", name, r)
		}
	}
}
