//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPromptAuditFallbackAccountCompatibilityUsesResolvedCompositePlatform(t *testing.T) {
	ctx := WithResolvedTargetPlatform(context.Background(), PlatformOpenAI)
	require.True(t, promptAuditFallbackAccountCompatible(ctx, PlatformComposite, PlatformOpenAI, ContentModerationProtocolOpenAIChat, "provider-specific-alias", false))
	require.False(t, promptAuditFallbackAccountCompatible(WithResolvedTargetPlatform(context.Background(), PlatformAnthropic), PlatformComposite, PlatformOpenAI, ContentModerationProtocolOpenAIChat, "provider-specific-alias", false))
}

func TestPromptAuditFallbackAccountCompatibilityHandlesGeminiMixedScheduling(t *testing.T) {
	require.True(t, promptAuditFallbackAccountCompatible(context.Background(), PlatformComposite, PlatformGemini, ContentModerationProtocolGemini, "gemini-2.5-pro", false))
	require.False(t, promptAuditFallbackAccountCompatible(context.Background(), PlatformComposite, PlatformAntigravity, ContentModerationProtocolGemini, "gemini-2.5-pro", false))
	require.True(t, promptAuditFallbackAccountCompatible(context.Background(), PlatformComposite, PlatformAntigravity, ContentModerationProtocolGemini, "gemini-2.5-pro", true))
}

func TestPromptAuditFallbackAccountSupportsModel(t *testing.T) {
	account := &Account{
		Platform: PlatformOpenAI,
		Credentials: map[string]any{
			CredentialKeyModelMappingRestricts: true,
			"model_mapping":                    map[string]any{"gpt-allowed": "gpt-upstream"},
		},
	}
	require.True(t, promptAuditFallbackAccountSupportsModel(account, "gpt-allowed"))
	require.False(t, promptAuditFallbackAccountSupportsModel(account, "gpt-blocked"))
}

func TestPromptAuditFallbackOpenAICapability(t *testing.T) {
	empty := PromptAuditFallbackRequirements{}
	require.Equal(t, OpenAIEndpointCapabilityEmbeddings, promptAuditFallbackOpenAICapability("openai_embeddings", PlatformOpenAI, empty))
	require.Equal(t, OpenAIEndpointCapabilityLive, promptAuditFallbackOpenAICapability("openai_live", PlatformOpenAI, empty))
	require.Equal(t, OpenAIEndpointCapabilityGrokMediaGeneration, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIImages, PlatformGrok, empty))
	require.Equal(t, OpenAIEndpointCapabilityChatCompletions, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIResponses, PlatformOpenAI, empty))
	require.Equal(t, OpenAIEndpointCapabilityResponses, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIResponses, PlatformOpenAI, PromptAuditFallbackRequirements{ImageIntent: true}))
	require.Equal(t, OpenAIEndpointCapabilityResponses, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIResponses, PlatformOpenAI, PromptAuditFallbackRequirements{RequireResponses: true}))
	require.Equal(t, OpenAIEndpointCapabilityChatCompletions, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIResponses, PlatformOpenAI, PromptAuditFallbackRequirements{RequireCompact: true}))
	require.Equal(t, OpenAIEndpointCapabilityChatCompletions, promptAuditFallbackOpenAICapability(ContentModerationProtocolOpenAIResponses, PlatformGrok, PromptAuditFallbackRequirements{ImageIntent: true}))
}
