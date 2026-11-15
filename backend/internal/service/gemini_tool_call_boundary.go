package service

import (
	"encoding/json"
	"strings"
)

// geminiToolCallBoundary tracks identity separately from the function name.
// Native object-valued args without IDs represent independent calls, including
// identical calls. String-valued fragments may end with a cumulative snapshot;
// preserve that snapshot's identity until its object-valued args arrive.
// Without IDs, a terminal snapshot and a new identical call immediately after
// fragments are indistinguishable; prefer the snapshot in that specific case.
type geminiToolCallBoundary struct {
	upstreamID string
	fragmented bool
}

func (b *geminiToolCallBoundary) startsNewBlock(openName, seenJSON, argsJSON string, fc map[string]any) bool {
	name, _ := fc["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "tool"
	}
	if name != openName {
		return true
	}
	id, _ := fc["id"].(string)
	if id != "" && b.upstreamID != "" {
		return id != b.upstreamID
	}
	if !json.Valid([]byte(seenJSON)) {
		return false
	}
	// Keep the existing delta accumulator's duplicate/rewind behavior for a
	// fragmented call. Do not deduplicate separate complete native calls.
	if b.fragmented && strings.HasPrefix(seenJSON, strings.TrimSuffix(argsJSON, "\u0000")) {
		return false
	}
	return true
}

func (b *geminiToolCallBoundary) observe(fc map[string]any, seenJSON string) {
	if b.upstreamID == "" {
		b.upstreamID, _ = fc["id"].(string)
	}
	if _, isString := fc["args"].(string); isString {
		b.fragmented = b.fragmented || !json.Valid([]byte(seenJSON))
	} else {
		// The terminal object snapshot has been consumed. A following
		// anonymous complete object can now be another identical call.
		b.fragmented = false
	}
}

func geminiToolCallArgsJSON(args any) string {
	switch v := args.(type) {
	case nil:
	case string:
		if strings.TrimSpace(v) != "" {
			return v
		}
	default:
		if b, err := json.Marshal(args); err == nil && len(b) > 0 {
			return string(b)
		}
	}
	return "{}"
}
