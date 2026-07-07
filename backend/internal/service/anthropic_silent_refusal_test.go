package service

import "testing"

func TestAnthropicRefusalDetector_IsSilentRefusal(t *testing.T) {
	refusalDelta := map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "refusal"},
	}
	endTurnDelta := map[string]any{
		"type":  "message_delta",
		"delta": map[string]any{"stop_reason": "end_turn"},
	}
	contentStart := map[string]any{"type": "content_block_start"}
	contentDelta := map[string]any{"type": "content_block_delta"}

	t.Run("refusal with no content", func(t *testing.T) {
		d := newAnthropicRefusalDetector(true)
		d.ObserveEvent("message_delta", refusalDelta)
		if !d.IsSilentRefusal() {
			t.Fatal("want silent refusal")
		}
	})

	t.Run("refusal after content is not silent", func(t *testing.T) {
		d := newAnthropicRefusalDetector(true)
		d.ObserveEvent("content_block_start", contentStart)
		d.ObserveEvent("content_block_delta", contentDelta)
		d.ObserveEvent("message_delta", refusalDelta)
		if d.IsSilentRefusal() {
			t.Fatal("content-bearing turn must not be a silent refusal")
		}
	})

	t.Run("end_turn no content is not refusal", func(t *testing.T) {
		d := newAnthropicRefusalDetector(true)
		d.ObserveEvent("message_delta", endTurnDelta)
		if d.IsSilentRefusal() {
			t.Fatal("end_turn must not be a silent refusal")
		}
	})

	t.Run("disabled never reports refusal", func(t *testing.T) {
		d := newAnthropicRefusalDetector(false)
		d.ObserveEvent("message_delta", refusalDelta)
		if d.IsSilentRefusal() {
			t.Fatal("disabled detector must not report refusal")
		}
		if d.Enabled() {
			t.Fatal("disabled detector must report not enabled")
		}
	})

	t.Run("nil safe", func(t *testing.T) {
		var d *anthropicRefusalDetector
		d.ObserveEvent("message_delta", refusalDelta)
		if d.IsSilentRefusal() || d.Enabled() || d.HasMeaningfulContent() {
			t.Fatal("nil detector must be inert")
		}
	})
}
