//go:build unit

package service

import (
	"testing"
)

func TestApplyThinkingModelSuffix(t *testing.T) {
	tests := []struct {
		name            string
		mappedModel     string
		thinkingEnabled bool
		expected        string
	}{
		// Thinking 未开启：保持原样
		{
			name:            "thinking disabled - claude-sonnet-4-5 unchanged",
			mappedModel:     "claude-sonnet-4-5",
			thinkingEnabled: false,
			expected:        "claude-sonnet-4-5",
		},
		{
			name:            "thinking disabled - other model unchanged",
			mappedModel:     "claude-opus-4-6-thinking",
			thinkingEnabled: false,
			expected:        "claude-opus-4-6-thinking",
		},

		// Thinking 开启 + claude-sonnet-4-5：自动添加后缀
		{
			name:            "thinking enabled - claude-sonnet-4-5 becomes thinking version",
			mappedModel:     "claude-sonnet-4-5",
			thinkingEnabled: true,
			expected:        "claude-sonnet-4-5-thinking",
		},

		// Thinking 开启 + 其他模型：保持原样
		{
			name:            "thinking enabled - claude-sonnet-4-5-thinking unchanged",
			mappedModel:     "claude-sonnet-4-5-thinking",
			thinkingEnabled: true,
			expected:        "claude-sonnet-4-5-thinking",
		},
		{
			name:            "thinking enabled - claude-opus-4-6-thinking unchanged",
			mappedModel:     "claude-opus-4-6-thinking",
			thinkingEnabled: true,
			expected:        "claude-opus-4-6-thinking",
		},
		{
			name:            "thinking enabled - gemini model unchanged",
			mappedModel:     "gemini-3-flash",
			thinkingEnabled: true,
			expected:        "gemini-3-flash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := applyThinkingModelSuffix(tt.mappedModel, tt.thinkingEnabled)
			if result != tt.expected {
				t.Errorf("applyThinkingModelSuffix(%q, %v) = %q, want %q",
					tt.mappedModel, tt.thinkingEnabled, result, tt.expected)
			}
		})
	}
}
