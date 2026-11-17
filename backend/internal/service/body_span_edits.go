package service

import "sort"

// jsonSpanEdit replaces body[start:end] with data. Spans are absolute byte
// offsets into the original body; edits are sorted before applying so callers
// can append them in whatever order they discover them.
type jsonSpanEdit struct {
	start int
	end   int
	data  []byte
}

// applyJSONSpanEdits materializes all edits in one pass. It returns ok=false
// when any span is out of range or overlapping, in which case the caller keeps
// the original body: a malformed edit plan must never produce corrupted JSON.
func applyJSONSpanEdits(body []byte, edits []jsonSpanEdit) ([]byte, bool) {
	if len(edits) == 0 {
		return body, false
	}
	sort.SliceStable(edits, func(i, j int) bool { return edits[i].start < edits[j].start })

	size := len(body)
	last := 0
	for _, edit := range edits {
		if edit.start < 0 || edit.end < edit.start || edit.end > len(body) || edit.start < last {
			return body, false
		}
		size += len(edit.data) - (edit.end - edit.start)
		last = edit.end
	}

	out := make([]byte, 0, size)
	cursor := 0
	for _, edit := range edits {
		out = append(out, body[cursor:edit.start]...)
		out = append(out, edit.data...)
		cursor = edit.end
	}
	out = append(out, body[cursor:]...)
	return out, true
}
