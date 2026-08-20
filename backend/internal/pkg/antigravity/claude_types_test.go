package antigravity

import "testing"

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
		"gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview",
		"gemini-3-pro-image", // legacy compatibility
		"gemini-3.6-flash",
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-low",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-tiered",
	}

	for _, id := range requiredIDs {
		if _, ok := byID[id]; !ok {
			t.Fatalf("expected model %q to be exposed in DefaultModels", id)
		}
	}
}

func TestDefaultModels_ContainsGemini37Tiered(t *testing.T) {
	t.Parallel()

	models := DefaultModels()
	byID := make(map[string]ClaudeModel, len(models))
	counts := make(map[string]int, len(models))
	for _, model := range models {
		byID[model.ID] = model
		counts[model.ID]++
	}
	want := map[string]string{
		"gemini-3.7-flash":        "Gemini 3.7 Flash",
		"gemini-3.7-flash-tiered": "Gemini 3.7 Flash Tiered",
	}
	for id, displayName := range want {
		if counts[id] != 1 {
			t.Fatalf("expected Antigravity model %q exactly once, got %d", id, counts[id])
		}
		if byID[id].DisplayName != displayName {
			t.Fatalf("unexpected display name for %q: got %q want %q", id, byID[id].DisplayName, displayName)
		}
	}
	for _, id := range []string{"gemini-3.7-flash-high", "gemini-3.7-flash-medium", "gemini-3.7-flash-low"} {
		if counts[id] != 0 {
			t.Fatalf("did not expect unverified Antigravity model %q, got %d entries", id, counts[id])
		}
	}
}
