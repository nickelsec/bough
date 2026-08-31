package claude

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRealCorpus runs the parser over whatever Claude Code history exists on
// the machine running the tests. It is skipped when there is none, so it stays
// useful for a contributor with their own history and harmless in CI.
//
// It asserts properties rather than numbers, since every developer's corpus is
// different. The numbers that matter are pinned in the fixture tests.
func TestRealCorpus(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	root := filepath.Join(home, ".claude", "projects")
	dirs, err := os.ReadDir(root)
	if err != nil {
		t.Skip("no Claude Code history on this machine")
	}

	var files []string
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		// Only top level transcripts. The same directory also holds subagents,
		// tool-results and memory, none of which are sessions.
		matches, _ := filepath.Glob(filepath.Join(root, d.Name(), "*.jsonl"))
		files = append(files, matches...)
	}
	if len(files) == 0 {
		t.Skip("no transcripts found")
	}

	for _, fp := range files {
		f, err := os.Open(fp)
		if err != nil {
			t.Errorf("open %s: %v", fp, err)
			continue
		}
		recs, err := ReadRecords(f)
		f.Close()
		if err != nil {
			t.Errorf("read %s: %v", fp, err)
			continue
		}

		seen := make(map[string]bool, len(recs))
		for _, r := range recs {
			if r.UUID == "" {
				continue
			}
			if seen[r.UUID] {
				t.Errorf("%s: uuid %s returned twice", filepath.Base(fp), r.UUID)
				break
			}
			seen[r.UUID] = true
		}

		// Tool results are filed as user records. If they are being counted as
		// prompts the ratio goes wrong by an order of magnitude, which is the
		// mistake this guards against.
		prompts, userRecs := 0, 0
		for _, r := range recs {
			if r.Type == "user" && !r.IsSidechain {
				userRecs++
			}
			if r.IsHumanPrompt() {
				prompts++
			}
		}
		if userRecs > 100 && prompts > userRecs/2 {
			t.Errorf("%s: %d prompts out of %d user records, tool results are leaking in",
				filepath.Base(fp), prompts, userRecs)
		}
	}
}
