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
		// 函数现在直接返回原值（所有 Sonnet 已在映射层统一到 Opus）
		{
			name:            "thinking disabled - model unchanged",
			mappedModel:     "claude-opus-4-6-thinking",
			thinkingEnabled: false,
			expected:        "claude-opus-4-6-thinking",
		},
		{
			name:            "thinking enabled - model unchanged",
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
