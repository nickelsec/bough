package claude

import (
	"os"
	"strings"
	"testing"
)

// The format is append-only with replay, so the first thing to prove is that
// reading a transcript yields records rather than lines.
func TestReadRecordsCollapsesReplay(t *testing.T) {
	f, err := os.Open("testdata/replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	recs, err := ReadRecords(f)
	if err != nil {
		t.Fatal(err)
	}

	// The fixture holds 340 lines covering 260 distinct records, 40 of which
	// were written twice. Reading it back must give the 260.
	const want = 260
	if len(recs) != want {
		t.Errorf("got %d records, want %d (line count must not leak through)", len(recs), want)
	}

	seen := make(map[string]bool, len(recs))
	for _, r := range recs {
		if r.UUID == "" {
			continue
		}
		if seen[r.UUID] {
			t.Errorf("uuid %s returned more than once", r.UUID)
		}
		seen[r.UUID] = true
	}
}

// Order matters for everything downstream, since segmentation walks turns in
// sequence. A merged duplicate must stay where it first appeared.
func TestReadRecordsKeepsFirstAppearanceOrder(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"uuid":"a","type":"user","timestamp":"2026-08-01T10:00:00.000Z"}`,
		`{"uuid":"b","type":"assistant","timestamp":"2026-08-01T10:01:00.000Z"}`,
		`{"uuid":"a","type":"user","promptId":"p1","timestamp":"2026-08-01T10:00:00.000Z"}`,
		`{"uuid":"c","type":"user","timestamp":"2026-08-01T10:02:00.000Z"}`,
	}, "\n"))

	recs, err := ReadRecords(in)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(recs))
	for i, r := range recs {
		got[i] = r.UUID
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Later copies of a record carry fields the first copy left empty. Those have
// to be picked up, or prompts lose the ids that group them.
func TestMergeTakesLaterValues(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"uuid":"a","type":"user","cwd":"d:\\proj"}`,
		`{"uuid":"a","type":"user","promptId":"p1","slug":"some-slug"}`,
	}, "\n"))

	recs, err := ReadRecords(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("got %d records, want 1", len(recs))
	}
	if recs[0].PromptID != "p1" {
		t.Errorf("promptId = %q, want p1", recs[0].PromptID)
	}
	if recs[0].Slug != "some-slug" {
		t.Errorf("slug = %q, want some-slug", recs[0].Slug)
	}
}

// A field missing from a later copy was simply not repeated. Treating that as a
// deletion would throw away the cwd and the timestamps.
func TestMergeKeepsEarlierWhenLaterIsEmpty(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"uuid":"a","type":"user","cwd":"d:\\proj","promptId":"p1"}`,
		`{"uuid":"a","type":"user"}`,
	}, "\n"))

	recs, err := ReadRecords(in)
	if err != nil {
		t.Fatal(err)
	}
	if recs[0].CWD != `d:\proj` {
		t.Errorf("cwd = %q, want %q", recs[0].CWD, `d:\proj`)
	}
	if recs[0].PromptID != "p1" {
		t.Errorf("promptId = %q, want p1", recs[0].PromptID)
	}
}

// One unreadable line should cost the user that line, not the session.
func TestReadRecordsSkipsMalformedLines(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"uuid":"a","type":"user"}`,
		`{ this is not json`,
		``,
		`{"uuid":"b","type":"assistant"}`,
	}, "\n"))

	recs, err := ReadRecords(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
}

// Records with no uuid cannot be identified or merged, so they are kept as they
// come rather than dropped.
func TestReadRecordsKeepsRecordsWithoutUUID(t *testing.T) {
	in := strings.NewReader(strings.Join([]string{
		`{"type":"queue-operation","operation":"enqueue"}`,
		`{"uuid":"a","type":"user"}`,
	}, "\n"))

	recs, err := ReadRecords(in)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2", len(recs))
	}
}

// Fixtures are cut from real transcripts, which contain working directories,
// personal paths and occasionally pasted credentials. This checks that the
// scrubbing held, so a future regeneration cannot quietly publish any of it.
func TestFixtureCarriesNoRealData(t *testing.T) {
	b, err := os.ReadFile("testdata/replay.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(b))

	for _, pattern := range []string{
		"sk-", "ghp_", "akia", "-----begin",
		"users/", `users\`, "/home/", "c:", "d:",
	} {
		if strings.Contains(body, pattern) {
			t.Errorf("fixture contains %q, it needs scrubbing before it can ship", pattern)
		}
	}
}
