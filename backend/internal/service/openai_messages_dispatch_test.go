package service

import "testing"

import "github.com/stretchr/testify/require"

func TestNormalizeOpenAIMessagesDispatchModelConfig(t *testing.T) {
	t.Parallel()

	cfg := normalizeOpenAIMessagesDispatchModelConfig(OpenAIMessagesDispatchModelConfig{
		FableMappedModel:  " mapped-fable ",
		OpusMappedModel:   " gpt-5.4-high ",
		SonnetMappedModel: "gpt-5.3-codex",
		HaikuMappedModel:  " gpt-5.4-mini-medium ",
		ExactModelMappings: map[string]string{
			" claude-sonnet-4-5-20250929 ": " gpt-5.2-high ",
			"":                             "gpt-5.4",
			"claude-opus-4-6":              " ",
		},
	})

	require.Equal(t, "mapped-fable", cfg.FableMappedModel)
	require.Equal(t, "gpt-5.4", cfg.OpusMappedModel)
	require.Equal(t, "gpt-5.3-codex", cfg.SonnetMappedModel)
	require.Equal(t, "gpt-5.4-mini", cfg.HaikuMappedModel)
	require.Equal(t, map[string]string{
		"claude-sonnet-4-5-20250929": "gpt-5.2",
	}, cfg.ExactModelMappings)
}

func TestGroupResolveMessagesDispatchModel_FableFamily(t *testing.T) {
	t.Parallel()

	group := &Group{MessagesDispatchModelConfig: OpenAIMessagesDispatchModelConfig{
		FableMappedModel: "mapped-fable",
		ExactModelMappings: map[string]string{
			"claude-fable-5-special": "mapped-special",
		},
	}}

	require.Equal(t, "mapped-fable", group.ResolveMessagesDispatchModel("claude-fable-5"))
	require.Equal(t, "mapped-special", group.ResolveMessagesDispatchModel("claude-fable-5-special"))
	require.Empty(t, (&Group{}).ResolveMessagesDispatchModel("claude-fable-5"))
}

func TestGroupResolveMessagesDispatchModel_GrokMapsClaudeFamilyToGrok(t *testing.T) {
	t.Parallel()

	group := &Group{Platform: PlatformGrok}

	require.Equal(t, "grok-4.5", group.ResolveMessagesDispatchModel("claude-sonnet-4-5"))
	require.Equal(t, "grok-4.5", group.ResolveMessagesDispatchModel("claude-fable-5"))
	require.Equal(t, "grok-4.5", group.ResolveMessagesDispatchModel("claude-opus-4-6"))
	require.Equal(t, "grok-4.5", group.ResolveMessagesDispatchModel("claude-haiku-4-5"))
	require.Empty(t, group.ResolveMessagesDispatchModel("grok"))
	require.Empty(t, group.ResolveMessagesDispatchModel("gpt-5.3-codex"))
}
