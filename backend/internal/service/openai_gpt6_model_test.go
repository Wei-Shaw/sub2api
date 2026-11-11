package service

import "testing"

func TestCodexModelNormalization_GPT6AndGPT56(t *testing.T) {
	cases := map[string]string{
		"gpt-6-sol":               "gpt-6-sol",
		"gpt-6-astra":             "gpt-6-astra",
		"gpt-6-luna":              "gpt-6-luna",
		"gpt-6.1-sol":             "gpt-6.1-sol",
		"gpt-6.1-terra":           "gpt-6.1-terra",
		"gpt-6-sol-high":          "gpt-6-sol",
		"gpt-6-sol-2026-09-22":    "gpt-6-sol",
		"gpt-5.6":                 "gpt-5.6-sol",
		"gpt-5.6-sol":             "gpt-5.6-sol",
		"gpt-5.6-terra":           "gpt-5.6-terra",
		"gpt-5.6-luna-2026-07-09": "gpt-5.6-luna",
		"openai/gpt-5.6-terra":    "gpt-5.6-terra",
		// 既有行为不变
		"gpt-5.5": "gpt-5.5",
		"gpt-5.4": "gpt-5.4",
		"gpt-5":   "gpt-5.4",
		"gpt-5.3": "gpt-5.3-codex",
	}
	for in, want := range cases {
		if got := normalizeCodexModel(in); got != want {
			t.Errorf("normalizeCodexModel(%q) = %q, want %q", in, got, want)
		}
	}
	// 非 OpenAI 模型不应被误识别
	for _, m := range []string{"grok-4.7", "kimi-k3", "deepseek-v4-pro", "glm-5.3", "claude-opus-5-5", "gemini-4-carbon"} {
		if got := normalizeKnownOpenAICodexModel(m); got != "" {
			t.Errorf("normalizeKnownOpenAICodexModel(%q) = %q, want empty", m, got)
		}
	}
}
