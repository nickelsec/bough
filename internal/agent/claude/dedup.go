package claude

import (
	"bufio"
	"encoding/json"
	"io"
)

// maxLine is the scanner buffer ceiling. Real transcripts contain single lines
// over a megabyte when a large tool result was inlined.
const maxLine = 16 << 20

// ReadRecords parses a transcript and collapses the replay duplicates.
//
// Claude Code appends to these files as a session is resumed or rewound, so the
// same record is written more than once. On the corpus this was built against,
// one 36,676 line file held only 10,964 distinct records, with some appearing
// six times. Counting lines instead of records overstated that session by more
// than three to one.
//
// Repeat copies are not identical. Later ones tend to have fields filled in
// that were empty the first time round, such as slug, promptId and the tool
// result. So the copies are merged rather than deduplicated, taking the later
// value wherever there is one, and the first appearance decides the ordering.
//
// Lines that will not parse are skipped. Partial history is still worth
// reading, and a single bad line should not cost the user their whole session.
func ReadRecords(r io.Reader) ([]*Record, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), maxLine)

	var order []string
	byUUID := make(map[string]*Record)
	var anon []*Record

	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}

		rec := &Record{}
		if err := json.Unmarshal(line, rec); err != nil {
			continue
		}

		// Records without a uuid cannot be identified, so they are kept as they
		// come. These are bookkeeping entries rather than conversation.
		if rec.UUID == "" {
			anon = append(anon, rec)
			continue
		}

		prev, seen := byUUID[rec.UUID]
		if !seen {
			order = append(order, rec.UUID)
			byUUID[rec.UUID] = rec
			continue
		}
		merge(prev, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}

	out := make([]*Record, 0, len(order)+len(anon))
	for _, u := range order {
		out = append(out, byUUID[u])
	}
	return append(out, anon...), nil
}

// merge folds a later copy of a record into the one already held.
//
// Later wins, but only where it actually says something. A field that arrives
// empty the second time is a field the writer did not repeat, not a field that
// was cleared, so an empty value never overwrites a populated one.
func merge(dst, src *Record) {
	setStr(&dst.ParentUUID, src.ParentUUID)
	setStr(&dst.SessionID, src.SessionID)
	setStr(&dst.PromptID, src.PromptID)
	setStr(&dst.Type, src.Type)
	setStr(&dst.Subtype, src.Subtype)
	setStr(&dst.Timestamp, src.Timestamp)
	setStr(&dst.CWD, src.CWD)
	setStr(&dst.Slug, src.Slug)
	setStr(&dst.AITitle, src.AITitle)
	setStr(&dst.Version, src.Version)

	if src.IsSidechain {
		dst.IsSidechain = true
	}
	if src.Message != nil {
		if dst.Message == nil || len(src.Message.Content.Blocks) > 0 || src.Message.Content.Text != "" {
			dst.Message = src.Message
		}
	}
}

func setStr(dst *string, src string) {
	if src != "" {
		*dst = src
	}
}
