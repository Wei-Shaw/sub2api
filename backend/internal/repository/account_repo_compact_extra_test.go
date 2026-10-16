package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_CompactCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_compact_supported":  true,
		"openai_compact_checked_at": "2026-04-10T10:00:00Z",
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected compact capability updates to enqueue scheduler outbox")
	}
}

func TestShouldEnqueueSchedulerOutboxForExtraUpdates_OpenAIResponsesCapabilityKeysAreRelevant(t *testing.T) {
	updates := map[string]any{
		"openai_responses_mode":      "force_chat_completions",
		"openai_responses_supported": false,
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected responses capability updates to enqueue scheduler outbox")
	}
}

// 能力记录变化必须重建调度快照，否则候选过滤读到的仍是旧能力。
func TestShouldEnqueueSchedulerOutboxForExtraUpdates_CodexCapabilitiesAreRelevant(t *testing.T) {
	updates := map[string]any{
		service.CodexCapabilitiesExtraKey: map[string]any{"service_tiers": []string{"priority"}},
	}

	if !shouldEnqueueSchedulerOutboxForExtraUpdates(updates) {
		t.Fatalf("expected codex capabilities updates to enqueue scheduler outbox")
	}
	if isSchedulerNeutralExtraKey(service.CodexCapabilitiesExtraKey) {
		t.Fatalf("codex capabilities must not be a scheduler-neutral key")
	}
}
