package service

import "github.com/Wei-Shaw/sub2api/internal/pkg/openai"

// HasCompactionTriggerInInput detects an input item with
// type="compaction_trigger". The handler combines this body signal with the
// request path, stream flag, and Codex beta feature header to distinguish the
// native remote compaction v2 wire from the legacy /responses/compact bridge.
func HasCompactionTriggerInInput(body []byte) bool {
	return openai.HasCompactionTriggerInInput(body)
}
