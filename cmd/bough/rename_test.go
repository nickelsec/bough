package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// renameEnv is testEnv with a settings folder of its own, so a test can keep
// names without touching the ones on the machine running it.
func renameEnv(t *testing.T, out, errs *bytes.Buffer) Env {
	t.Helper()
	env := testEnv(out, errs)
	env.Settings = t.TempDir()
	return env
}

func TestARenamedProjectShowsItsNewNameEverywhere(t *testing.T) {
	root := twoProjects(t, "so-i-have-basically", "other")
	var out, errs bytes.Buffer
	env := renameEnv(t, &out, &errs)

	if err := run([]string{"so-i-have", "--root", root, "--rename", "Portfolio site"}, env); err != nil {
		t.Fatalf("renaming failed: %v", err)
	}

	out.Reset()
	if err := run([]string{"--root", root, "--list"}, env); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Portfolio site") {
		t.Errorf("the list does not show the new name:\n%s", out.String())
	}

	out.Reset()
	if err := run([]string{"--root", root, "portfolio", "--json"}, env); err != nil {
		t.Fatalf("the new name does not find the project: %v", err)
	}
	var g struct {
		Project struct {
			Name string `json:"name"`
		} `json:"project"`
	}
	if err := json.Unmarshal(out.Bytes(), &g); err != nil {
		t.Fatal(err)
	}
	if g.Project.Name != "Portfolio site" {
		t.Errorf("the JSON names it %q", g.Project.Name)
	}
}

// Anything that used the old name, a habit or a script, keeps working.
func TestARenamedProjectStillAnswersToItsFolder(t *testing.T) {
	root := twoProjects(t, "so-i-have-basically", "other")
	var out, errs bytes.Buffer
	env := renameEnv(t, &out, &errs)

	if err := run([]string{"so-i-have", "--root", root, "--rename", "Portfolio site"}, env); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"so-i-have", "--root", root, "--text"}, env); err != nil {
		t.Fatalf("the folder's name no longer finds the project: %v", err)
	}
	if !strings.Contains(out.String(), "Portfolio site") {
		t.Errorf("expected the project under its new name:\n%s", out.String())
	}
}

func TestAnEmptyRenameGivesTheFolderNameBack(t *testing.T) {
	root := twoProjects(t, "example", "other")
	var out, errs bytes.Buffer
	env := renameEnv(t, &out, &errs)

	if err := run([]string{"example", "--root", root, "--rename", "Shiny"}, env); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"shiny", "--root", root, "--rename", ""}, env); err != nil {
		t.Fatalf("resetting failed: %v", err)
	}
	out.Reset()
	if err := run([]string{"--root", root, "--list"}, env); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Shiny") || !strings.Contains(out.String(), "example") {
		t.Errorf("the folder's name did not come back:\n%s", out.String())
	}
}

func TestRenamingNeedsAProject(t *testing.T) {
	root := twoProjects(t, "example", "other")
	var out, errs bytes.Buffer
	if err := run([]string{"--root", root, "--rename", "X"}, renameEnv(t, &out, &errs)); err == nil {
		t.Error("a rename with no project named was accepted")
	}
}
