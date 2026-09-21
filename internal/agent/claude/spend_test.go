package claude

import (
	"encoding/json"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// reply builds one assistant record carrying a usage block, written as the
// JSON a transcript actually holds so the field names under test are the ones
// being parsed rather than ones restated in Go.
func reply(t *testing.T, id, model, usage string) *Record {
	t.Helper()
	var m Message
	body := `{"role":"assistant","id":"` + id + `","model":"` + model + `","usage":` + usage + `}`
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("building a reply: %v", err)
	}
	return &Record{
		Timestamp: "2026-09-08T12:00:01Z",
		Type:      "assistant",
		UUID:      id + "-uuid",
		Message:   &m,
	}
}

func prompt(t *testing.T, text string) *Record {
	t.Helper()
	var m Message
	body := `{"role":"user","content":"` + text + `"}`
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		t.Fatalf("building a prompt: %v", err)
	}
	return &Record{
		Timestamp: "2026-09-08T12:00:00Z",
		Type:      "user",
		UUID:      "u-" + text,
		Message:   &m,
	}
}

// What a model was charged for is kept in full, not just what it wrote.
//
// The four counts are priced at rates that differ by a factor of fifty, so a
// per-model figure that holds only output tokens cannot be turned into a bill.
func TestEachModelKeepsItsWholeBill(t *testing.T) {
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "m1", "claude-opus-5", `{
			"input_tokens": 10,
			"output_tokens": 20,
			"cache_read_input_tokens": 300,
			"cache_creation_input_tokens": 40
		}`),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	got := turns[0].Models["claude-opus-5"]
	want := agent.Tokens{Input: 10, Output: 20, CacheRead: 300, CacheWrite: 40}
	if got != want {
		t.Errorf("charged %+v, want %+v", got, want)
	}
}

// The per-model counts sum to the turn's own counts, at every level. This is
// the invariant a bill rests on: if the two ever drift, one of the numbers on
// screen is wrong and there is no way to tell which.
func TestModelSpendSumsToTheTurn(t *testing.T) {
	recs := []*Record{
		prompt(t, "one"),
		reply(t, "m1", "claude-opus-5", `{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":300,"cache_creation_input_tokens":40}`),
		reply(t, "m2", "claude-haiku-4-5", `{"input_tokens":1,"output_tokens":2,"cache_read_input_tokens":3,"cache_creation_input_tokens":4}`),
	}

	turns := ExtractTurns(recs)
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1", len(turns))
	}
	var sum agent.Tokens
	for _, t := range turns[0].Models {
		sum.Add(t)
	}
	if sum != turns[0].Tokens {
		t.Errorf("models sum to %+v, turn holds %+v", sum, turns[0].Tokens)
	}
}

// One reply arrives as several records and must be charged once.
//
// The guard keys on the message id rather than the record's own uuid, because
// the id is what is shared between them. Pricing makes this worse than it was:
// a tripled count used to read as a large number, and now reads as a large
// number of dollars.
func TestOneReplyIsChargedOnce(t *testing.T) {
	usage := `{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":300,"cache_creation_input_tokens":40}`
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "same-id", "claude-opus-5", usage),
		reply(t, "same-id", "claude-opus-5", usage),
		reply(t, "same-id", "claude-opus-5", usage),
	}

	turns := ExtractTurns(recs)
	want := agent.Tokens{Input: 10, Output: 20, CacheRead: 300, CacheWrite: 40}
	if got := turns[0].Tokens; got != want {
		t.Errorf("turn charged %+v, want %+v: one reply billed more than once", got, want)
	}
	if got := turns[0].Models["claude-opus-5"]; got != want {
		t.Errorf("model charged %+v, want %+v", got, want)
	}
}

// Storing context for an hour costs more than storing it for five minutes, and
// the flat cache_creation_input_tokens field does not say which it was.
//
// Reading only that field and pricing it at the cheaper rate put this
// project's bill 4.7% under the published rates. Claude Code writes the
// hourly cache for effectively everything, so the error was on nearly the
// whole of that line.
func TestHourlyCacheIsReadFromTheBreakdown(t *testing.T) {
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "m1", "claude-opus-5", `{
			"input_tokens": 1,
			"output_tokens": 2,
			"cache_read_input_tokens": 3,
			"cache_creation_input_tokens": 100,
			"cache_creation": {
				"ephemeral_5m_input_tokens": 30,
				"ephemeral_1h_input_tokens": 70
			}
		}`),
	}

	turns := ExtractTurns(recs)
	tk := turns[0].Tokens
	if tk.CacheWrite != 100 {
		t.Errorf("cache write = %d, want 100", tk.CacheWrite)
	}
	if tk.CacheWriteHour != 70 {
		t.Errorf("hourly cache write = %d, want 70", tk.CacheWriteHour)
	}
	// The hourly part is inside the flat figure, so the total is unchanged by
	// reading it. Counting it separately would inflate every token count.
	if tk.Total() != 1+2+3+100 {
		t.Errorf("total = %d, want %d: the hourly slice was counted twice", tk.Total(), 106)
	}
}

// A record with no breakdown is charged at the ordinary rate rather than
// guessed at. Older transcripts predate the field.
func TestNoBreakdownMeansNoHourlyClaim(t *testing.T) {
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "m1", "claude-opus-5", `{"output_tokens":2,"cache_creation_input_tokens":100}`),
	}
	if got := ExtractTurns(recs)[0].Tokens.CacheWriteHour; got != 0 {
		t.Errorf("hourly = %d, want 0 when the record does not say", got)
	}
}

// A breakdown that claims more than the flat figure must not let the dearer
// rate be charged on tokens that were never written.
func TestHourlyCacheCannotExceedTheFlatFigure(t *testing.T) {
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "m1", "claude-opus-5", `{
			"output_tokens": 2,
			"cache_creation_input_tokens": 50,
			"cache_creation": {"ephemeral_1h_input_tokens": 500}
		}`),
	}
	tk := ExtractTurns(recs)[0].Tokens
	if tk.CacheWriteHour > tk.CacheWrite {
		t.Errorf("hourly %d exceeds the %d actually written", tk.CacheWriteHour, tk.CacheWrite)
	}
}

// A harness placeholder is not a model anyone chose, so it takes no share of
// the bill. Its tokens still count towards the turn.
func TestSyntheticTakesNoShareOfTheBill(t *testing.T) {
	recs := []*Record{
		prompt(t, "do it"),
		reply(t, "m1", "<synthetic>", `{"output_tokens":5}`),
	}
	turns := ExtractTurns(recs)
	if len(turns[0].Models) != 0 {
		t.Errorf("models = %v, want none", turns[0].Models)
	}
	if turns[0].Tokens.Output != 5 {
		t.Errorf("output = %d, want 5: the tokens were still spent", turns[0].Tokens.Output)
	}
}
