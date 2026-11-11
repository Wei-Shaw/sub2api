package service

import (
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeClaudeThinkingForModel(t *testing.T) {
	cases := []struct {
		name      string
		model     string
		body      string
		wantRaw   string // 期望的 thinking 字段原始 JSON；空字符串表示字段应被删除
		unchanged bool
	}{
		{"opus55 enabled→adaptive", "claude-opus-5-5", `{"thinking":{"type":"enabled","budget_tokens":2048}}`, `{"type":"adaptive"}`, false},
		{"opus55 disabled→removed", "claude-opus-5-5", `{"thinking":{"type":"disabled"}}`, "", false},
		{"fable52 disabled→removed", "claude-fable-5-2", `{"thinking":{"type":"disabled"}}`, "", false},
		{"sonnet55 disabled→between_tools", "claude-sonnet-5-5", `{"thinking":{"type":"disabled"}}`, `{"type":"between_tools"}`, false},
		{"sonnet55 disabled xhigh→removed", "claude-sonnet-5-5", `{"thinking":{"type":"disabled"},"output_config":{"effort":"xhigh"}}`, "", false},
		{"opus5 disabled high→kept", "claude-opus-5", `{"thinking":{"type":"disabled"},"output_config":{"effort":"high"}}`, `{"type":"disabled"}`, true},
		{"opus5 disabled max→removed", "claude-opus-5", `{"thinking":{"type":"disabled"},"output_config":{"effort":"max"}}`, "", false},
		{"haiku55 enabled→adaptive", "claude-haiku-5-5", `{"thinking":{"type":"enabled","budget_tokens":1024}}`, `{"type":"adaptive"}`, false},
		{"adaptive strips stray budget", "claude-sonnet-5", `{"thinking":{"type":"adaptive","budget_tokens":1024}}`, `{"type":"adaptive"}`, false},
		{"opus48 disabled kept", "claude-opus-4-8", `{"thinking":{"type":"disabled"}}`, `{"type":"disabled"}`, true},
		{"sonnet46 untouched", "claude-sonnet-4-6", `{"thinking":{"type":"enabled","budget_tokens":2048}}`, `{"type":"enabled","budget_tokens":2048}`, true},
		{"no thinking untouched", "claude-opus-5-5", `{"max_tokens":10}`, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := normalizeClaudeThinkingForModel([]byte(c.body), c.model)
			if c.unchanged && string(out) != c.body {
				t.Fatalf("body changed: %s", out)
			}
			got := gjson.GetBytes(out, "thinking")
			if c.wantRaw == "" {
				if got.Exists() {
					t.Fatalf("thinking should be removed, got %s", got.Raw)
				}
				return
			}
			if got.Raw != c.wantRaw {
				t.Fatalf("thinking = %s, want %s", got.Raw, c.wantRaw)
			}
		})
	}
}

func TestSanitizeBedrockThinking_Gen5(t *testing.T) {
	// Sonnet 5.5 以前会被补上 budget_tokens 导致 400
	out := sanitizeBedrockThinking([]byte(`{"thinking":{"type":"enabled"}}`), "anthropic.claude-sonnet-5-5")
	if got := gjson.GetBytes(out, "thinking").Raw; got != `{"type":"adaptive"}` {
		t.Fatalf("sonnet 5.5: thinking = %s", got)
	}
	// Opus 5（无 minor）以前无法解析版本
	out = sanitizeBedrockThinking([]byte(`{"thinking":{"type":"enabled","budget_tokens":4096}}`), "anthropic.claude-opus-5")
	if got := gjson.GetBytes(out, "thinking").Raw; got != `{"type":"adaptive"}` {
		t.Fatalf("opus 5: thinking = %s", got)
	}
	if !isBedrockClaude45OrNewer("anthropic.claude-opus-5") {
		t.Fatal("opus 5 should support cache ttl")
	}
}
