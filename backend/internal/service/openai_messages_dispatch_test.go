package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveMessagesDispatchModel_DefaultMappedModelFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		group          *Group
		requestedModel string
		want           string
	}{
		{
			name: "opus with no config uses hardcoded default",
			group: &Group{
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{},
			},
			requestedModel: "claude-opus-4-5",
			want:           defaultOpenAIMessagesDispatchOpusMappedModel,
		},
		{
			name: "opus with DefaultMappedModel uses group default as fallback",
			group: &Group{
				DefaultMappedModel:          "gpt-5.3-codex",
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{},
			},
			requestedModel: "claude-opus-4-5",
			want:           "gpt-5.3-codex",
		},
		{
			name: "opus with explicit OpusMappedModel ignores DefaultMappedModel",
			group: &Group{
				DefaultMappedModel: "gpt-5.3-codex",
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
					OpusMappedModel: "gpt-5.4",
				},
			},
			requestedModel: "claude-opus-4-5",
			want:           "gpt-5.4",
		},
		{
			name: "sonnet with DefaultMappedModel uses group default as fallback",
			group: &Group{
				DefaultMappedModel:          "gpt-5.3-codex",
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{},
			},
			requestedModel: "claude-sonnet-4-5",
			want:           "gpt-5.3-codex",
		},
		{
			name: "haiku with DefaultMappedModel uses group default as fallback",
			group: &Group{
				DefaultMappedModel:          "gpt-5.3-codex",
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{},
			},
			requestedModel: "claude-haiku-3-5",
			want:           "gpt-5.3-codex",
		},
		{
			name: "exact model mapping takes precedence over DefaultMappedModel",
			group: &Group{
				DefaultMappedModel: "gpt-5.3-codex",
				MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
					ExactModelMappings: map[string]string{
						"claude-opus-4-5": "gpt-5.2",
					},
				},
			},
			requestedModel: "claude-opus-4-5",
			want:           "gpt-5.2",
		},
		{
			name:           "nil group returns empty",
			group:          nil,
			requestedModel: "claude-opus-4-5",
			want:           "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.group.ResolveMessagesDispatchModel(tt.requestedModel)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	t.Parallel()

	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		OpusMappedModel:   " gpt-5.4-high ",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  " gpt-5.4-mini-medium ",
		ExactModelMappings: map[string]string{
			" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			"":                             "gpt-5.4",
			"claude-opus-4-6":              " ",
		},
	})

	require.Equal(t, "gpt-5.4", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, "gpt-5.4-mini", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": "gpt-5.2",
	}, cfg.ExactModelMappings)
}
