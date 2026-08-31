package rollup

import (
	"sort"

	"github.com/nickelsec/bough/internal/metrics"
)

// Link joins two goals that worked on the same files.
//
// Goals are sittings, so they are contiguous by construction. That is honest
// about when the work happened but it loses something real: coming back to the
// parser three days later is connected to the first attempt, and a plain
// timeline cannot say so. Links carry that connection without pretending the
// two sittings were one.
//
// This is deliberately not a parent relationship. Neither goal owns the other,
// and drawing these as hierarchy would be the lie a strict tree tells.
type Link struct {
	// From and To index into the goals, always with From before To in time.
	From, To int

	// Files are the shared files, most worked first.
	Files []string

	// Weight is how much shared work sits behind the link, counted as the
	// smaller number of edits on each side so one busy sitting cannot inflate
	// a link to a sitting that barely touched the file.
	Weight int
}

// LinkOptions tune which connections are worth drawing.
type LinkOptions struct {
	// MinWeight is the least shared work a link needs. One incidental edit on
	// each side is usually a passing visit rather than resumed work.
	MinWeight int
}

// DefaultLinkOptions are the fitted defaults.
func DefaultLinkOptions() LinkOptions {
	return LinkOptions{MinWeight: 2}
}

// Links finds the goals that returned to the same work.
//
// On real history these come out sparse, five links across seven goals in one
// project and none at all in two others, so there is no thicket to prune. Where
// there is nothing to say, nothing is drawn.
func Links(goals []Goal, opt LinkOptions) []Link {
	if len(goals) < 2 {
		return nil
	}

	edited := make([]map[string]int, len(goals))
	for i, g := range goals {
		edited[i] = map[string]int{}
		for _, t := range g.Turns() {
			for f, n := range t.Edits {
				// The agent's own plan and memory files are touched in nearly
				// every sitting, so counting them would connect everything to
				// everything and say nothing.
				if metrics.Ambient(f) {
					continue
				}
				edited[i][f] += n
			}
		}
	}

	var links []Link
	for i := range goals {
		for j := i + 1; j < len(goals); j++ {
			shared, weight := overlap(edited[i], edited[j])
			if weight < opt.MinWeight || len(shared) == 0 {
				continue
			}
			links = append(links, Link{From: i, To: j, Files: shared, Weight: weight})
		}
	}

	sort.Slice(links, func(a, b int) bool {
		if links[a].Weight != links[b].Weight {
			return links[a].Weight > links[b].Weight
		}
		if links[a].From != links[b].From {
			return links[a].From < links[b].From
		}
		return links[a].To < links[b].To
	})
	return links
}

// overlap returns the files two goals both changed, and how much shared work
// that represents.
func overlap(a, b map[string]int) ([]string, int) {
	var shared []string
	weight := 0
	for f, an := range a {
		bn, ok := b[f]
		if !ok {
			continue
		}
		shared = append(shared, f)
		if an < bn {
			weight += an
		} else {
			weight += bn
		}
	}
	// Most worked first, then by name so the order never wanders.
	sort.Slice(shared, func(i, j int) bool {
		ai, aj := a[shared[i]], a[shared[j]]
		if ai != aj {
			return ai > aj
		}
		return shared[i] < shared[j]
	})
	return shared, weight
}
