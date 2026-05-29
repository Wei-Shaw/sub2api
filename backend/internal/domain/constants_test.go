package domain

import "testing"

func TestDefaultAntigravityModelMapping_ImageCompatibilityAliases(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"gemini-2.5-flash-image":         "gemini-2.5-flash-image",
		"gemini-2.5-flash-image-preview": "gemini-2.5-flash-image",
		"gemini-3.1-flash-image":         "gemini-3.1-flash-image",
		"gemini-3.1-flash-image-preview": "gemini-3.1-flash-image",
		"gemini-3-pro-image":             "gemini-3.1-flash-image",
		"gemini-3-pro-image-preview":     "gemini-3.1-flash-image",
	}

	for from, want := range cases {
		got, ok := DefaultAntigravityModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestPlatformKiroConstant(t *testing.T) {
	t.Parallel()
	if PlatformKiro != "kiro" {
		t.Fatalf("PlatformKiro = %q, want %q", PlatformKiro, "kiro")
	}
}

func TestDefaultKiroModelMapping_CoreModels(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"claude-sonnet-4-6":          "claude-sonnet-4.6",
		"claude-sonnet-4-6-thinking": "claude-sonnet-4.6",
		"claude-sonnet-4-5":          "claude-sonnet-4.5",
		"claude-sonnet-4-5-thinking": "claude-sonnet-4.5",
		"claude-sonnet-4":            "claude-sonnet-4",
		"claude-opus-4-7":            "claude-opus-4.7",
		"claude-opus-4-6":            "claude-opus-4.6",
		"claude-opus-4-6-thinking":   "claude-opus-4.6",
		"claude-haiku-4-5":           "claude-haiku-4.5",
	}
	for from, want := range cases {
		got, ok := DefaultKiroModelMapping[from]
		if !ok {
			t.Fatalf("expected mapping for %q to exist", from)
		}
		if got != want {
			t.Fatalf("unexpected mapping for %q: got %q want %q", from, got, want)
		}
	}
}

func TestDefaultAntigravityModelMapping_ContainsOpus48(t *testing.T) {
	t.Parallel()

	got, ok := DefaultAntigravityModelMapping["claude-opus-4-8"]
	if !ok {
		t.Fatal("expected mapping for claude-opus-4-8 to exist")
	}
	if got != "claude-opus-4-8" {
		t.Fatalf("unexpected claude-opus-4-8 mapping: got %q", got)
	}
}

func TestDefaultBedrockModelMapping_ContainsOpus48(t *testing.T) {
	t.Parallel()

	got, ok := DefaultBedrockModelMapping["claude-opus-4-8"]
	if !ok {
		t.Fatal("expected Bedrock mapping for claude-opus-4-8 to exist")
	}
	if got != "us.anthropic.claude-opus-4-8-v1" {
		t.Fatalf("unexpected Bedrock claude-opus-4-8 mapping: got %q", got)
	}
}
