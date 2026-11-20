package service

import (
	"encoding/json"
	"strings"
)

// geminiToolCallBoundary tracks identity separately from the function name.
// Native object-valued args without IDs represent independent calls, including
// identical calls. String-valued arguments use stream mode even when the first
// chunk is already complete JSON; retain duplicate strings and the terminal
// object snapshot in that call. Without IDs, a terminal snapshot and a new
// identical call after strings are indistinguishable; prefer the snapshot.
type geminiToolCallBoundary struct {
	upstreamID string
	stringArgs bool
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
	// string-argument call. Do not deduplicate separate complete native calls.
	if b.stringArgs && strings.HasPrefix(seenJSON, strings.TrimSuffix(argsJSON, "\u0000")) {
		return false
	}
	return true
}

func (b *geminiToolCallBoundary) observe(fc map[string]any) {
	if b.upstreamID == "" {
		b.upstreamID, _ = fc["id"].(string)
	}
	// The representation determines the mode, not whether the first string
	// happens to contain complete JSON. An object consumes the final snapshot,
	// allowing the next anonymous native object to be an independent call.
	_, b.stringArgs = fc["args"].(string)
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
