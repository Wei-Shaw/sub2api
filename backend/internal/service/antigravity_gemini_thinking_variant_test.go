package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func newAntigravityAccountWithMapping(mapping map[string]string) *Account {
	mappingAny := make(map[string]any, len(mapping))
	for k, v := range mapping {
		mappingAny[k] = v
	}
	return &Account{
		ID:       1,
		Name:     "ag",
		Platform: PlatformAntigravity,
		Credentials: map[string]any{
			"model_mapping": mappingAny,
		},
	}
}

func TestGeminiThinkingLevelFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty body", ``, "high"},
		{"no thinkingConfig", `{"contents":[]}`, "high"},
		{"budget -1 dynamic", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":-1}}}`, "high"},
		{"budget 0 off", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":0}}}`, "low"},
		{"budget 1000 (agy low)", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":1000}}}`, "low"},
		{"budget 4000 (agy medium)", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":4000}}}`, "medium"},
		{"budget 24576", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":24576}}}`, "high"},
		{"thinkingLevel wins over budget", `{"generationConfig":{"thinkingConfig":{"thinkingBudget":24576,"thinkingLevel":"low"}}}`, "low"},
		{"thinkingLevel medium", `{"generationConfig":{"thinkingConfig":{"thinkingLevel":"MEDIUM"}}}`, "medium"},
		{"invalid json", `{"generationConfig":`, "high"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, geminiThinkingLevelFromBody([]byte(tt.body)))
		})
	}
}

func TestResolveGeminiThinkingVariant(t *testing.T) {
	// 与生产 Antigravity 账号从上游同步来的目录一致：只有带后缀的变体，没有裸名。
	catalog := map[string]string{
		"gemini-3.8-flash-low":    "gemini-3.8-flash-low",
		"gemini-3.8-flash-medium": "gemini-3.8-flash-medium",
		"gemini-3.8-flash-high":   "gemini-3.8-flash-high",
		"gemini-3.8-flash-tiered": "gemini-3.8-flash-tiered",
		"gemini-3.6-flash-high":   "gemini-3.6-flash-high",
		"gemini-3.1-pro-high":     "gemini-3.1-pro-high",
		"gemini-2.5-flash":        "gemini-2.5-flash",
	}
	budget := func(v string) []byte {
		return []byte(`{"contents":[],"generationConfig":{"thinkingConfig":{"thinkingBudget":` + v + `}}}`)
	}

	tests := []struct {
		name      string
		mapping   map[string]string
		model     string
		body      []byte
		want      string
		wantMatch bool
	}{
		{"agy high (-1) -> -high", catalog, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-high", true},
		{"agy medium (4000) -> -medium", catalog, "gemini-3.8-flash", budget("4000"), "gemini-3.8-flash-medium", true},
		{"agy low (1000) -> -low", catalog, "gemini-3.8-flash", budget("1000"), "gemini-3.8-flash-low", true},
		{"models/ prefix stripped", catalog, "models/gemini-3.8-flash", budget("1000"), "gemini-3.8-flash-low", true},
		{"no thinkingConfig -> -high", catalog, "gemini-3.8-flash", []byte(`{"contents":[]}`), "gemini-3.8-flash-high", true},
		// gemini-3.6/3.7/3.8-flash 的四个变体会被 resolveModelMapping 自动补齐，所以降级用一个不在默认表里的型号
		{"only -high exists: low request degrades to high", map[string]string{
			"gemini-3.5-flash-high": "gemini-3.5-flash-high",
		}, "gemini-3.5-flash", budget("1000"), "gemini-3.5-flash-high", true},
		{"injected default variants are usable", catalog, "gemini-3.6-flash", budget("1000"), "gemini-3.6-flash-low", true},
		{"already suffixed: untouched", catalog, "gemini-3.8-flash-low", budget("-1"), "", false},
		{"bare name explicitly mapped: untouched", catalog, "gemini-2.5-flash", budget("-1"), "", false},
		{"no variants in mapping: untouched", catalog, "gemini-9.9-flash", budget("-1"), "", false},
		{"non-gemini model: untouched", catalog, "claude-sonnet-4-6", budget("-1"), "", false},
		{"explicit bare alias wins over variants", map[string]string{
			"gemini-3.8-flash":      "gemini-3.8-flash-tiered",
			"gemini-3.8-flash-high": "gemini-3.8-flash-high",
		}, "gemini-3.8-flash", budget("-1"), "", false},
		{"variant value is itself a mapping target", map[string]string{
			"gemini-3.8-flash-high": "gemini-3.8-flash-tiered",
		}, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-tiered", true},
		{"user-written bare identity passthrough: respected", map[string]string{
			"gemini-3.8-flash":      "gemini-3.8-flash",
			"gemini-3.8-flash-high": "gemini-3.8-flash-high",
		}, "gemini-3.8-flash", budget("1000"), "", false},
		{"runtime-injected bare passthrough (not in credentials): still resolved", map[string]string{
			// resolveModelMapping 会补 gemini-3.7-flash → gemini-3.7-flash 的默认透传，
			// 但 credentials 里没有这条，应当继续推导变体。
			"gemini-3.7-flash-medium": "gemini-3.7-flash-medium",
		}, "gemini-3.7-flash", budget("4000"), "gemini-3.7-flash-medium", true},
		// 空 credentials 映射 → 走 DefaultAntigravityModelMapping，其中同样只有带后缀的 3.8 flash
		{"empty mapping falls back to default catalog", map[string]string{}, "gemini-3.8-flash", budget("-1"), "gemini-3.8-flash-high", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			account := newAntigravityAccountWithMapping(tt.mapping)
			got, matched := resolveGeminiThinkingVariant(account, tt.model, tt.body)
			require.Equal(t, tt.wantMatch, matched)
			require.Equal(t, tt.want, got)
		})
	}

	t.Run("nil account", func(t *testing.T) {
		got, matched := resolveGeminiThinkingVariant(nil, "gemini-3.8-flash", budget("-1"))
		require.False(t, matched)
		require.Empty(t, got)
	})
}
