package service

import (
	"encoding/json"
	"strings"
)

// geminiToolCallStartsNewBlock distinguishes call identity from function name.
// Calls with explicit IDs may contain incremental or cumulative arguments.
// Without IDs on both parts, a complete argument document marks the boundary
// before the next call, even when its name and arguments are identical.
func geminiToolCallStartsNewBlock(openID, openName, seenJSON string, fc map[string]any) bool {
	name, _ := fc["name"].(string)
	if strings.TrimSpace(name) == "" {
		name = "tool"
	}
	if name != openName {
		return true
	}
	id, _ := fc["id"].(string)
	if id != "" && openID != "" {
		return id != openID
	}
	return json.Valid([]byte(seenJSON))
}
