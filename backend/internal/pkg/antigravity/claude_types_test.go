package antigravity

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

func TestDefaultModels_ContainsNewAndLegacyImageModels(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]ClaudeModel, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	requiredIDs := []string{
		"claude-opus-4-6-thinking",
		"claude-sonnet-4-6",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-2.5-pro",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3.1-flash-lite",
		"gemini-3-flash-agent",
		"gemini-3-pro-image", // legacy compatibility
		"gemini-pro-agent",
		"gpt-oss-120b-medium",
		"tab_flash_lite_preview",
		"tab_jump_flash_lite_preview",
		"gemini-3.5-flash-low",
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
		"gemini-3.7-flash",
		"gemini-3.7-flash-high",
		"gemini-3.7-flash-low",
		"gemini-3.7-flash-medium",
		"gemini-3.7-flash-tiered",
	}

	retiredIDs := []string{"claude-fable-5", "claude-opus-4-7", "claude-opus-4-8"}
	for _, id := range retiredIDs {
		if _, ok := byID[id]; ok {
			t.Fatalf("retired model %q must not appear in DefaultModels", id)
		}
	}

	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected model %q to be exposed in DefaultModels", id)
		}
	}
}

// DefaultModels must expose every client-facing key from the default mapping so
// /v1/models and /antigravity/models stay aligned with schedulable models (#3701).
func TestDefaultModels_CoversDefaultAntigravityMappingKeys(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]struct{}, len(models))
	for _, m := range models {
		byID[m.ID] = struct{}{}
	}

	for id := range domain.DefaultAntigravityModelMapping {
		if _, ok := byID[id]; !ok {
			t.Fatalf("DefaultModels missing mapping key %q (issue #3701)", id)
		}
	}
}
