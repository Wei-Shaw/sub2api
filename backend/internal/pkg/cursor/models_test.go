package cursor

import "testing"

func TestDefaultModelsIncludesCursorPickerFallbacks(t *testing.T) {
	ids := DefaultModelIDs()
	for _, want := range []string{
		"default",
		"grok-4.6",
		"composer-2.5",
		"claude-opus-5",
		"gpt-5.6-sol",
		"grok-4.5",
		"claude-opus-4-8",
		"gpt-5.5",
		"claude-fable-5",
		"gemini-3.7-flash",
		"gpt-5.6-terra",
		"claude-sonnet-5",
		"claude-sonnet-4-6",
		"gpt-5.3-codex",
		"gpt-5.4-mini",
		"kimi-k3",
		"kimi-k2.7-code",
		"composer-1",
	} {
		if !containsString(ids, want) {
			t.Fatalf("DefaultModelIDs missing %q: %v", want, ids)
		}
	}
}

func TestResolveModelAliases(t *testing.T) {
	tests := []struct {
		in       string
		want     string
		fallback bool
	}{
		{in: "claude-opus-5", want: "claude-opus-5", fallback: false},
		{in: "opus", want: "claude-opus-5", fallback: true},
		{in: "auto", want: "default", fallback: true},
		{in: "GPT-5.6 Terra", want: "gpt-5.6-terra", fallback: true},
		{in: "gpt-5.6", want: "gpt-5.6-sol", fallback: true},
		{in: "codex", want: "gpt-5.3-codex", fallback: true},
		{in: "Cursor Grok 4.6", want: "grok-4.6", fallback: true},
		{in: "openai/gpt-5.4-mini", want: "gpt-5.4-mini", fallback: true},
		{in: "unknown-model", want: "unknown-model", fallback: false},
		{in: "  composer  ", want: "composer-2.5", fallback: true},
	}
	for _, tt := range tests {
		got, fallback := ResolveModel(tt.in)
		if got != tt.want || fallback != tt.fallback {
			t.Fatalf("ResolveModel(%q) = %q, %v; want %q, %v", tt.in, got, fallback, tt.want, tt.fallback)
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
