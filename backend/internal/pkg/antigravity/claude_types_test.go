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
		"claude-fable-5",
		"claude-opus-4-8",
		"claude-opus-4-6-thinking",
		"gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview",
		"gemini-2.5-pro",
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image", // legacy compatibility
		"gemini-pro-agent",
		"gpt-oss-120b-medium",
		"tab_flash_lite_preview",
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
