package minimax

import "testing"

func TestIsM31(t *testing.T) {
	for _, model := range []string{ModelM31FlashPreview, " minimax-m3.1-flash-preview ", "minimax/MiniMax-M3.1-Flash-Preview"} {
		if !IsM31(model) {
			t.Errorf("model %q should match M3.1", model)
		}
	}
	for _, model := range []string{"MiniMax-M3", "MiniMax-M2.7", "MiniMax-M3.10-Flash-Preview", "MiniMax-M3.1", "claude-sonnet-4"} {
		if IsM31(model) {
			t.Errorf("model %q should not match M3.1", model)
		}
	}
}

func TestValidateM31Reasoning(t *testing.T) {
	for _, effort := range append(ReasoningEfforts(), "") {
		if err := ValidateReasoningEffort(ModelM31FlashPreview, effort); err != nil {
			t.Fatal(err)
		}
	}
	for _, effort := range []string{"none", "auto", "minimal", "ultracode"} {
		if err := ValidateReasoningEffort(ModelM31FlashPreview, effort); err == nil {
			t.Errorf("effort %q should be rejected", effort)
		}
		if err := ValidateReasoningEffort("MiniMax-M3", effort); err != nil {
			t.Errorf("M3 should retain existing behavior: %v", err)
		}
	}
	for _, thinking := range []string{"", "enabled", "adaptive"} {
		if err := ValidateThinkingType(ModelM31FlashPreview, thinking); err != nil {
			t.Fatal(err)
		}
	}
	if err := ValidateThinkingType(ModelM31FlashPreview, "disabled"); err == nil {
		t.Fatal("M3.1 thinking cannot be disabled")
	}
	if err := ValidateThinkingType("MiniMax-M2.7", "disabled"); err != nil {
		t.Fatal(err)
	}
}
