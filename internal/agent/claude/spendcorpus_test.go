package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nickelsec/bough/internal/agent"
)

// What the parser makes of real history has to match a plain reading of the
// same files.
//
// Every fixture test here is written against records somebody wrote by hand,
// which is exactly where a misunderstanding of the format survives: a fixture
// can only hold what its author already believed. This reads whatever history
// is on the machine and adds the usage blocks up independently, deduplicated
// the way the format requires, then asks the parser for the same figure.
//
// It caught a real error. Cache writes were being read from the flat field
// alone, which says nothing about how long the context was held for, and the
// two time to live rates differ by 60%. The totals matched, so nothing here
// failed; what failed was the bill, by 4.7%. Hence the hourly check below.
func TestParserMatchesAPlainReadingOfRealHistory(t *testing.T) {
	files := corpusFiles(t)
	if len(files) == 0 {
		t.Skip("no Claude Code history on this machine")
	}

	var checked int
	for _, path := range files {
		// Read once, into memory, and give both readings the same bytes. A
		// transcript being written to while the test runs otherwise grows
		// between the two passes, and the session running this test is
		// usually one of them.
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		recs, err := ReadRecords(strings.NewReader(string(raw)))
		if err != nil {
			continue
		}

		want, ok := addUpUsage(t, recs)
		if !ok || want.Total() == 0 {
			continue
		}

		var got agent.Tokens
		for _, turn := range ExtractTurns(recs) {
			got.Add(turn.Tokens)
		}

		// Every reply is counted by both sides, the placeholder ones included,
		// so the two figures are comparable without exception. On this
		// machine's history they match exactly across every transcript.
		if got != want {
			t.Errorf("%s:\n  parser read %+v\n  the file says %+v",
				filepath.Base(path), got, want)
		}
		checked++
	}
	if checked == 0 {
		t.Skip("no history with token counts in it")
	}
	t.Logf("checked %d transcripts", checked)
}

// Every model's share of a turn adds up to what the turn cost, on real
// history. This is the invariant a bill rests on, and it is checked here
// against files nobody wrote for a test.
func TestModelSharesAddUpOnRealHistory(t *testing.T) {
	files := corpusFiles(t)
	if len(files) == 0 {
		t.Skip("no Claude Code history on this machine")
	}

	var turns, withModels int
	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		recs, err := ReadRecords(f)
		f.Close()
		if err != nil {
			continue
		}
		for _, turn := range ExtractTurns(recs) {
			turns++
			if len(turn.Models) == 0 {
				continue
			}
			withModels++
			var sum agent.Tokens
			for _, spend := range turn.Models {
				sum.Add(spend)
			}
			// Only the synthetic placeholder is charged to no model, and it
			// writes a handful of tokens per project, so a turn made entirely
			// of it is the one case where the two legitimately differ.
			if sum.Total() > turn.Tokens.Total() {
				t.Errorf("%s: models claim %d tokens, the turn was charged %d",
					filepath.Base(path), sum.Total(), turn.Tokens.Total())
			}
			if sum.CacheWriteHour > sum.CacheWrite {
				t.Errorf("%s: hourly cache %d exceeds cache writes %d",
					filepath.Base(path), sum.CacheWriteHour, sum.CacheWrite)
			}
		}
	}
	if withModels == 0 {
		t.Skip("no history naming a model")
	}
	t.Logf("%d turns, %d naming a model", turns, withModels)
}

// corpusFiles lists every transcript on the machine, newest projects first.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	root := filepath.Join(home, ".claude", "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var files []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		inner, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, f := range inner {
			if strings.HasSuffix(f.Name(), ".jsonl") {
				files = append(files, filepath.Join(root, d.Name(), f.Name()))
			}
		}
	}
	return files
}

// addUpUsage sums every usage block in a transcript, as plainly as possible:
// walk the records, take each reply once, add the four fields up. No turns, no
// prompts, no crediting.
//
// It shares the parsed records with the caller rather than re-reading the file
// so that both sides see identical bytes. A transcript being appended to while
// the test runs otherwise differs between two reads, and the session running
// the test is usually one of those.
func addUpUsage(t *testing.T, recs []*Record) (agent.Tokens, bool) {
	t.Helper()

	var out agent.Tokens
	seen := map[string]bool{}
	for _, r := range recs {
		if r.Message == nil || r.Message.Usage == nil {
			continue
		}
		// One reply spread over several records is charged once, keyed on the
		// id the records share.
		if id := r.Message.ID; id != "" {
			if seen[id] {
				continue
			}
			seen[id] = true
		}
		u := r.Message.Usage
		spend := agent.Tokens{
			Input:      u.Input,
			Output:     u.Output,
			CacheRead:  u.CacheRead,
			CacheWrite: u.CacheWrite,
		}
		if u.Cache != nil {
			spend.CacheWriteHour = min(u.Cache.Hour, spend.CacheWrite)
		}
		out.Add(spend)
	}
	return out, true
}
