package codex

import (
	"testing"
	"time"
)

// Times belong to the day the person was sitting at, not to UTC.
//
// Agents write timestamps with a Z suffix and time.Parse hands those back in
// UTC. East of Greenwich that is a different calendar day for a good part of
// every evening: a prompt typed at 01:57 in Asia/Calcutta is 20:27 the day
// before in UTC, and the diagram headed it with yesterday's date.
//
// Durations are the same either way, so segmenting never noticed and every
// test passed. It is the day a piece of work belongs to that was wrong, which
// is the thing the reader is actually looking at.
func TestTimesComeBackInLocalTime(t *testing.T) {
	r := &Record{Timestamp: "2026-09-09T20:27:47.202Z"}
	got := r.Time()

	//nolint:gosmopolitan // the local zone is the thing under test.
	if got.Location() != time.Local {
		t.Errorf("location = %s, want the local zone", got.Location())
	}

	// The instant itself must not move. This is a change of how the time is
	// presented, not of when it happened.
	want := time.Date(2026, 9, 9, 20, 27, 47, 202000000, time.UTC)
	if !got.Equal(want) {
		t.Errorf("time = %s, want the same instant as %s", got, want)
	}
}

// A missing or malformed timestamp still comes back as the zero time rather
// than as the zero time shifted into some zone, which would no longer be zero.
func TestBadTimestampsStayZero(t *testing.T) {
	for _, in := range []string{"", "not a time", "2026-13-45T99:99:99Z"} {
		if got := (&Record{Timestamp: in}).Time(); !got.IsZero() {
			t.Errorf("Time(%q) = %s, want the zero time", in, got)
		}
	}
}
