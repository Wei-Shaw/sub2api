package openai

import "strings"

// IsResponsesMetadataEvent identifies lifecycle metadata and protocol heartbeats.
// Treat unknown events as output when deciding whether retry is still safe.
func IsResponsesMetadataEvent(eventType string) bool {
	switch strings.TrimSpace(eventType) {
	// Queuing and moderation metadata do not commit model output or disarm the
	// first-output timeout; a following overload must remain safe to retry.
	case "response.created", "response.queued", "response.in_progress", "response.metadata",
		"codex.rate_limits", "codex.response.metadata", "keepalive":
		return true
	default:
		return false
	}
}
