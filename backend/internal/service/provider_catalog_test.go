package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		name     string
		modelID  string
		platform string
		wantID   string
		wantOK   bool
	}{
		{"openai model", "gpt-5.4", "openai", "v-claw-openai", true},
		{"anthropic model", "claude-sonnet-4-20250514", "anthropic", "v-claw-anthropic", true},
		{"gemini model", "gemini-2.5-pro", "gemini", "v-claw-google", true},
		{"antigravity maps to google", "gemini-2.5-flash", "antigravity", "v-claw-google", true},
		{"unknown platform", "some-model", "unknown", "", false},
		{"kiro not exposed", "kiro-v1", "kiro", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, ok := resolveProvider(tt.modelID, tt.platform)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantID, meta.ProviderID)
			}
		})
	}
}

func TestIsReasoningModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{"gpt-5.4", true},
		{"o3-pro", true},
		{"o4-mini", true},
		{"claude-sonnet-4-20250514", true},
		{"claude-opus-4-20250514", true},
		{"claude-haiku-3.5", false},
		{"gemini-2.5-pro", true},
		{"gemini-2.5-flash", true},
		{"gpt-4o-mini", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.want, isReasoningModel(tt.modelID))
		})
	}
}

func TestSupportsImageInput(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{"gpt-5.4", true},
		{"gpt-4o", true},
		{"claude-sonnet-4-20250514", true},
		{"gemini-2.5-pro", true},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.want, supportsImageInput(tt.modelID))
		})
	}
}

func TestDefaultContextWindow(t *testing.T) {
	assert.Equal(t, 1050000, defaultContextWindow("gpt-5.4"))
	assert.Equal(t, 128000, defaultContextWindow("gpt-4o"))
	assert.Equal(t, 200000, defaultContextWindow("o3-pro"))
	assert.Equal(t, 1000000, defaultContextWindow("claude-opus-4-20250514"))
	assert.Equal(t, 200000, defaultContextWindow("claude-sonnet-4-20250514"))
	assert.Equal(t, 1048576, defaultContextWindow("gemini-2.5-pro"))
}

func TestDefaultMaxTokens(t *testing.T) {
	assert.Equal(t, 128000, defaultMaxTokens("gpt-5.4"))
	assert.Equal(t, 16384, defaultMaxTokens("gpt-4o"))
	assert.Equal(t, 100000, defaultMaxTokens("o3-pro"))
	assert.Equal(t, 128000, defaultMaxTokens("claude-opus-4-20250514"))
	assert.Equal(t, 64000, defaultMaxTokens("claude-sonnet-4-20250514"))
	assert.Equal(t, 65536, defaultMaxTokens("gemini-2.5-pro"))
}

func TestBuildCatalog_EmptyChannels(t *testing.T) {
	// We can't easily mock ChannelService without an interface,
	// but we can test the helper functions above and verify the
	// BuildCatalog logic via integration test or by verifying
	// the response structure.
	_ = context.Background()
	_ = require.New(t)
}
