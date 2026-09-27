package server

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// When a sitting ran and for how long are shown in the viewer's own format, so
// the page's helpers are run under node against the shipped bough.js and
// checked for what they say rather than how. See timecheck.go.
func TestTimeHelpersHoldUp(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not available")
	}
	dir := t.TempDir()
	page, err := assets.ReadFile("bough.js")
	if err != nil {
		t.Fatal(err)
	}
	js := filepath.Join(dir, "bough.js")
	if err := os.WriteFile(js, page, 0o600); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "check.js")
	if err := os.WriteFile(script, []byte(timeCheck), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("node", script, js).CombinedOutput()
	if err != nil {
		t.Fatalf("time checks failed:\n%s", out)
	}
	t.Logf("\n%s", out)
}
