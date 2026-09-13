package securityaudit

import (
	"encoding/json"
	"net/http"
	"strings"
	"unicode/utf8"
)

var promptRecordingEndpoints = map[string]struct{}{
	"/v1/responses":        {},
	"/v1/messages":         {},
	"/v1/chat/completions": {},
}

func promptRecordSessionID(headers http.Header) string {
	for _, name := range []string{"Session-Id", "X-Claude-Code-Session-Id"} {
		if value := truncatePromptRecordIdentifier(strings.TrimSpace(headers.Get(name)), 128); value != "" {
			return value
		}
	}
	return ""
}

func truncatePromptRecordIdentifier(value string, maxBytes int) string {
	if len(value) <= maxBytes {
		return value
	}
	for maxBytes > 0 && !utf8.RuneStart(value[maxBytes]) {
		maxBytes--
	}
	return value[:maxBytes]
}

type codexTurnMetadata struct {
	RequestKind  string `json:"request_kind"`
	ThreadSource string `json:"thread_source"`
	TurnTrigger  string `json:"turn_trigger"`
}

// ShouldRecordPromptRequest limits prompt history to user-facing text APIs and
// excludes Codex background traffic identified in production request metadata.
func ShouldRecordPromptRequest(req Request) bool {
	if _, allowed := promptRecordingEndpoints[strings.TrimSpace(req.Endpoint)]; !allowed {
		return false
	}
	for _, raw := range req.Headers.Values("X-Codex-Turn-Metadata") {
		var metadata codexTurnMetadata
		if json.Unmarshal([]byte(raw), &metadata) != nil {
			continue
		}
		requestKind := strings.ToLower(strings.TrimSpace(metadata.RequestKind))
		threadSource := strings.ToLower(strings.TrimSpace(metadata.ThreadSource))
		turnTrigger := strings.ToLower(strings.TrimSpace(metadata.TurnTrigger))
		if requestKind == "memory" || requestKind == "compaction" {
			return false
		}
		if threadSource == "memory_consolidation" || threadSource == "thread_title" || threadSource == "system" || turnTrigger == "thread_title" {
			return false
		}
	}
	return true
}
