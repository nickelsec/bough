package claude

import (
	"bytes"
	"strings"
	"testing"
)

// FuzzReadRecords checks that no transcript, however malformed, can crash the
// parser. History files are written by another program and get truncated by
// crashes and full disks, so the reader has to survive anything.
func FuzzReadRecords(f *testing.F) {
	f.Add(`{"uuid":"a","type":"user","promptId":"p","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`)
	f.Add(`{"uuid":"a","type":"user","message":{"role":"user","content":"plain string"}}`)
	f.Add(`{"uuid":"a","type":"system","subtype":"compact_boundary"}`)
	f.Add(`{"uuid":"a","type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Edit","input":{"file_path":"/x.go"}}]}}`)
	f.Add(`{"uuid":"a"}` + "\n" + `{"uuid":"a","promptId":"p2"}`)
	f.Add(`{}`)
	f.Add(``)
	f.Add(`not json`)

	f.Fuzz(func(t *testing.T, body string) {
		recs, err := ReadRecords(strings.NewReader(body))
		if err != nil {
			return
		}
		// Whatever came back has to be safe to walk, since everything
		// downstream does exactly this.
		turns := ExtractTurns(recs)
		for _, turn := range turns {
			_ = turn.Text
			for range turn.Files {
			}
			for range turn.Edits {
			}
			for range turn.Delegated {
			}
		}

		// Records must never repeat, no matter what the input looked like.
		seen := make(map[string]bool, len(recs))
		for _, r := range recs {
			if r.UUID == "" {
				continue
			}
			if seen[r.UUID] {
				t.Fatalf("uuid %q returned twice", r.UUID)
			}
			seen[r.UUID] = true
		}
	})
}

// A line longer than the scanner's buffer must not silently truncate the rest
// of the file. Real transcripts inline tool output that runs into megabytes.
func TestReadRecordsHandlesVeryLongLines(t *testing.T) {
	huge := strings.Repeat("x", 2<<20)
	var buf bytes.Buffer
	buf.WriteString(`{"uuid":"a","type":"user","promptId":"p","message":{"role":"user","content":[{"type":"text","text":"` + huge + `"}]}}` + "\n")
	buf.WriteString(`{"uuid":"b","type":"user","promptId":"p2","message":{"role":"user","content":[{"type":"text","text":"after the big one"}]}}` + "\n")

	recs, err := ReadRecords(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("got %d records, want 2; a long line must not swallow what follows", len(recs))
	}
}

// A file cut off mid-write, which is what a crash leaves behind.
func TestReadRecordsHandlesTruncatedFinalLine(t *testing.T) {
	body := `{"uuid":"a","type":"user","promptId":"p","message":{"role":"user","content":[{"type":"text","text":"complete"}]}}` + "\n" +
		`{"uuid":"b","type":"user","promptId":"p2","mess`

	recs, err := ReadRecords(strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Errorf("got %d records, want the one complete record", len(recs))
	}
}
