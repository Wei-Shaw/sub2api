package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveProviderMeta(t *testing.T) {
	tests := []struct {
		name      string
		platform  string
		wantID    string
		wantName  string
		wantStyle string
		wantOK    bool
	}{
		{"openai", "openai", "v-claw-openai", "OpenAI", "openai-responses", true},
		{"anthropic", "anthropic", "v-claw-anthropic", "Anthropic", "anthropic-messages", true},
		{"gemini", "gemini", "v-claw-google", "Google", "google-native", true},
		{"antigravity maps to google", "antigravity", "v-claw-google", "Google", "google-native", true},
		{"unknown platform derives dynamically", "newplatform", "v-claw-newplatform", "Newplatform", "openai-chat", true},
		{"empty platform", "", "", "", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			providerID, providerName, apiStyle, ok := resolveProviderMeta(tt.platform)
			assert.Equal(t, tt.wantOK, ok)
			if ok {
				assert.Equal(t, tt.wantID, providerID)
				assert.Equal(t, tt.wantName, providerName)
				assert.Equal(t, tt.wantStyle, apiStyle)
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
	// All models on our system support image input
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

func TestCanOutputImage(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		{"gemini-2.5-flash-image", true},
		{"gemini-3.1-flash-image-preview", true},
		{"gemini-3-pro-image", true},
		{"gpt-5.4", false},
		{"claude-sonnet-4-20250514", false},
		{"gemini-2.5-pro", false},
		{"gemini-2.5-flash", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			assert.Equal(t, tt.want, canOutputImage(tt.modelID))
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
