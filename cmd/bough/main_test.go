package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// history writes a small transcript that looks like the real thing.
func history(t *testing.T, project string) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "d--"+project)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		`{"uuid":"1","type":"user","sessionId":"s1","promptId":"p1","cwd":"/work/` + project + `","timestamp":"2026-08-01T09:00:00.000Z",` +
			`"message":{"role":"user","content":[{"type":"text","text":"rework the export path so an embedded font renders on the first page"}]}}`,
		`{"uuid":"2","type":"assistant","timestamp":"2026-08-01T09:05:00.000Z","message":{"role":"assistant","content":[` +
			`{"type":"tool_use","name":"Edit","input":{"file_path":"/work/` + project + `/export.go"}}]}}`,
		`{"type":"ai-title","aiTitle":"Fixing the exporter","sessionId":"s1"}`,
	}
	if err := os.WriteFile(filepath.Join(dir, "s1.jsonl"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestListShowsProjects(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"--list", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "example") {
		t.Errorf("listing did not mention the project:\n%s", out.String())
	}
}

func TestJSONOutputIsValidAndVersioned(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--json", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if parsed["schema"] == nil {
		t.Error("output carries no schema version, so a reader cannot tell what it is looking at")
	}
}

// Flags have to work on either side of the project name. The standard parser
// stops at the first non-flag, which would quietly ignore the flag and print
// the wrong thing.
func TestFlagsWorkAfterTheProjectName(t *testing.T) {
	root := history(t, "example")

	var before, after, errs bytes.Buffer
	if err := run([]string{"--json", "--root", root, "example"}, &before, &errs); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"example", "--json", "--root", root}, &after, &errs); err != nil {
		t.Fatal(err)
	}

	for _, b := range []*bytes.Buffer{&before, &after} {
		var parsed map[string]any
		if err := json.Unmarshal(b.Bytes(), &parsed); err != nil {
			t.Fatalf("expected JSON either way, got: %s", b.String())
		}
	}
}

func TestTextOutputIsReadable(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--root", root}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	body := out.String()
	for _, want := range []string{"example", "prompt", "rework the export path"} {
		if !strings.Contains(body, want) {
			t.Errorf("text output is missing %q:\n%s", want, body)
		}
	}
}

// Someone with no history should get an explanation, not a stack trace or an
// empty screen.
func TestMissingHistoryExplainsItself(t *testing.T) {
	var out, errs bytes.Buffer
	err := run([]string{"--root", filepath.Join(t.TempDir(), "nothing")}, &out, &errs)

	if err == nil {
		t.Fatal("expected an error when there is no history")
	}
	if !strings.Contains(err.Error(), "no Claude Code history") {
		t.Errorf("error should say what was looked for, got: %v", err)
	}
}

func TestUnknownProjectSuggestsList(t *testing.T) {
	root := history(t, "example")
	var out, errs bytes.Buffer

	err := run([]string{"nonsense", "--root", root}, &out, &errs)
	if err == nil {
		t.Fatal("expected an error for an unknown project")
	}
	if !strings.Contains(err.Error(), "--list") {
		t.Errorf("error should point at --list, got: %v", err)
	}
}

func TestWritesToAFile(t *testing.T) {
	root := history(t, "example")
	dest := filepath.Join(t.TempDir(), "graph.json")
	var out, errs bytes.Buffer

	if err := run([]string{"example", "--json", "--root", root, "-o", dest}, &out, &errs); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Errorf("file does not hold valid JSON: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("nothing should go to the screen when writing to a file, got: %s", out.String())
	}
}
