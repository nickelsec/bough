package names

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamesSurviveReopening(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Set(`D:\work\so-i-have-basically-added-the`, "Portfolio site"); err != nil {
		t.Fatal(err)
	}

	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Spelled differently, but the same folder.
	got, ok := again.Name("d:/work/so-i-have-basically-added-the")
	if !ok || got != "Portfolio site" {
		t.Fatalf("got %q, %v; want the name back from disk", got, ok)
	}
}

func TestAnEmptyNameBringsBackTheFolders(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.Set("/home/u/app", "App")
	if err := s.Set("/home/u/app", "   "); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Name("/home/u/app"); ok {
		t.Fatal("a blank name should forget the old one")
	}
	again, _ := Open(dir)
	if _, ok := again.Name("/home/u/app"); ok {
		t.Fatal("the forgetting should reach the disk")
	}
}

func TestCleanKeepsANameToOneTidyLine(t *testing.T) {
	cases := map[string]string{
		"  spaced  out  ":      "spaced out",
		"two\nlines":           "two lines",
		"bell\x07 and nul\x00": "bell and nul",
		"":                     "",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
	long := strings.Repeat("é", MaxLen+10)
	if got := []rune(Clean(long)); len(got) != MaxLen {
		t.Errorf("a long name kept %d characters, want %d", len(got), MaxLen)
	}
}

// A file bough cannot read may be a newer bough's, or the person's own edit
// gone wrong. Either way it is not bough's to overwrite.
func TestADamagedFileIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, File)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(dir)
	if err == nil {
		t.Fatal("a damaged file should be reported")
	}
	if _, ok := s.Name("/x"); ok {
		t.Fatal("a damaged file should read as no names")
	}
	if err := s.Set("/x", "X"); err == nil {
		t.Fatal("writing over a damaged file should be refused")
	}
	data, _ := os.ReadFile(path)
	if string(data) != "{not json" {
		t.Fatalf("the damaged file was changed to %q", data)
	}
}

func TestNoFileIsNoNames(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "not-yet"))
	if err != nil {
		t.Fatalf("a missing file is not an error: %v", err)
	}
	if _, ok := s.Name("/x"); ok {
		t.Fatal("expected no names")
	}
	// And the folder is made on the first write.
	if err := s.Set("/x", "X"); err != nil {
		t.Fatal(err)
	}
}
