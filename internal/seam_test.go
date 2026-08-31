package internal_test

import (
	"os/exec"
	"strings"
	"testing"
)

// The core is only reusable across agents if the parts above the boundary never
// learn which agent they came from. Claude Code is the only source today, so
// nothing stops someone reaching into it for a field that happens to be handy,
// and the cost of that only shows up when a second agent is added.
//
// Everything from a normalised turn upward has to stay clean.
func TestCoreDoesNotDependOnAnyAgent(t *testing.T) {
	independent := []string{
		"github.com/nickelsec/bough/internal/segment",
		"github.com/nickelsec/bough/internal/rollup",
		"github.com/nickelsec/bough/internal/metrics",
	}

	for _, pkg := range independent {
		out, err := exec.Command("go", "list", "-deps", pkg).Output()
		if err != nil {
			t.Skipf("go list unavailable: %v", err)
		}
		for _, dep := range strings.Fields(string(out)) {
			if strings.Contains(dep, "/internal/agent/") {
				t.Errorf("%s depends on %s; it must only see normalised turns", pkg, dep)
			}
		}
	}
}
