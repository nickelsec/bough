package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// piHistory copies the sessions Pi's own SessionManager wrote into root, in
// the folder Pi would keep them in.
func piHistory(t *testing.T, root string) {
	t.Helper()
	dir := filepath.Join(root, "--C--work-app--")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"main", "fork", "clone"} {
		data, err := os.ReadFile(filepath.Join("..", "..", "internal", "agent", "pi", "testdata", "sessions", f+".jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, f+".jsonl"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAgentFlagPi(t *testing.T) {
	root := t.TempDir()
	piHistory(t, root)
	var out, errs bytes.Buffer
	if err := run([]string{"--list", "--agent", "pi", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "app") || !strings.Contains(out.String(), "[Pi]") {
		t.Errorf("listing did not show the Pi project:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "7 prompts") {
		t.Errorf("listing miscounted: a fork's copied prompts are not its own\n%s", out.String())
	}
}

// One --root reaches every agent, and Pi's folders look just like Claude
// Code's. Each agent must claim its own history and nothing else, or the same
// work is counted twice and an empty project appears besides.
func TestOneRootHoldsAllThreeAgents(t *testing.T) {
	root := t.TempDir()
	addHistory(t, root, "example")
	piHistory(t, root)
	codex := filepath.Join(root, "2026", "09", "09")
	if err := os.MkdirAll(codex, 0o755); err != nil {
		t.Fatal(err)
	}
	rollout := `{"timestamp":"2026-09-09T00:00:00Z","type":"session_meta","payload":{"id":"s1","cwd":"/w/rollup"}}` + "\n" +
		`{"timestamp":"2026-09-09T00:01:00Z","type":"response_item","payload":{"type":"message","role":"user",` +
		`"content":[{"type":"input_text","text":"a prompt long enough to count as a request rather than a nudge"}]}}` + "\n"
	if err := os.WriteFile(filepath.Join(codex, "rollout-2026-09-09T00-00-00-aaaa.jsonl"), []byte(rollout), 0o644); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	if err := run([]string{"--list", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d projects, want one per agent:\n%s", len(lines), out.String())
	}
	want := map[string]string{"example": "[Claude Code]", "rollup": "[Codex]", "app": "[Pi]"}
	for _, l := range lines {
		name := strings.Fields(l)[0]
		badge, ok := want[name]
		if !ok || !strings.Contains(l, badge) {
			t.Errorf("unexpected line %q", l)
		}
		delete(want, name)
	}
	if len(want) != 0 {
		t.Errorf("missing %v:\n%s", want, out.String())
	}
}

// The JSON carries everything a Pi session holds, checked field by field
// against what the fixture was written with.
func TestPiJSON(t *testing.T) {
	root := t.TempDir()
	piHistory(t, root)
	var out, errs bytes.Buffer
	if err := run([]string{"app", "--agent", "pi", "--json", "--no-repo", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	var g struct {
		Project struct {
			Agent     string `json:"agent"`
			AgentName string `json:"agentName"`
			Sessions  int    `json:"sessions"`
		} `json:"project"`
		Totals struct {
			Turns    int                        `json:"turns"`
			Edits    int                        `json:"edits"`
			Files    int                        `json:"files"`
			Errors   int                        `json:"errors"`
			Cost     *float64                   `json:"cost"`
			Unpriced []string                   `json:"unpriced"`
			Models   map[string]json.RawMessage `json:"models"`
		} `json:"totals"`
		Goals []struct {
			Stats struct {
				Commits []struct {
					SHA  string `json:"sha"`
					Kind string `json:"kind"`
				} `json:"commits"`
			} `json:"stats"`
		} `json:"goals"`
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	if g.Project.Agent != "pi" || g.Project.AgentName != "Pi" || g.Project.Sessions != 3 {
		t.Errorf("project = %+v", g.Project)
	}
	// Seven prompts: five in the main session and one each after the fork and
	// the clone. Three edits, four files, and two failures: a commit with
	// nothing to commit and a sub-agent's failing test run.
	tt := g.Totals
	if tt.Turns != 7 || tt.Edits != 3 || tt.Files != 4 || tt.Errors != 2 {
		t.Errorf("totals: %d turns, %d edits, %d files, %d errors", tt.Turns, tt.Edits, tt.Files, tt.Errors)
	}
	if tt.Cost == nil || *tt.Cost <= 0 || len(tt.Unpriced) != 0 {
		t.Errorf("cost = %v, unpriced %v", tt.Cost, tt.Unpriced)
	}
	for _, m := range []string{"gpt-5.6-luna", "gpt-5.6-sol", "claude-opus-4-8", "openrouter/nvidia/nemotron-3-ultra-550b-a55b:free"} {
		if _, ok := tt.Models[m]; !ok {
			t.Errorf("no %s among %v", m, tt.Models)
		}
	}
	var shas []string
	for _, goal := range g.Goals {
		for _, c := range goal.Stats.Commits {
			shas = append(shas, c.SHA+" "+c.Kind)
		}
	}
	if strings.Join(shas, ",") != "85fd4d9 committed,cd7d300 amended" {
		t.Errorf("commits = %v", shas)
	}
}

func TestPiText(t *testing.T) {
	root := t.TempDir()
	piHistory(t, root)
	var out, errs bytes.Buffer
	if err := run([]string{"app", "--agent", "pi", "--text", "-v", "--no-repo", "--root", root}, testEnv(&out, &errs)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"7 prompts", "at API rates", "/skill:review src/main.go please", "handed off: run the tests", "after the fork"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("text output lacks %q:\n%s", want, out.String())
		}
	}
}
