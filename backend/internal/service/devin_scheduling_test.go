//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevinSchedulingCapabilities(t *testing.T) {
	account := &Account{ID: 1, Platform: PlatformDevin, Type: AccountTypeOAuth, Status: StatusActive, Schedulable: true, Credentials: map[string]any{"access_token": "session", "model_mapping": map[string]any{"client-model": "swe-1-6"}}}
	require.True(t, account.IsOpenAICompatible())
	require.False(t, account.CanGetUsage())
	usage, err := (&AccountUsageService{}).GetUsageForAccount(context.Background(), account, true)
	require.NoError(t, err)
	require.Nil(t, usage.FiveHour)
	require.Contains(t, usage.Error, "unavailable")
	require.True(t, shouldForwardOpenAIResponsesViaRawChatCompletions(account))
	require.True(t, shouldEstimateOpenAIInputTokensLocally(account))
	require.True(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), account, PlatformDevin, "client-model", false, OpenAIEndpointCapabilityChatCompletions))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), account, PlatformOpenAI, "client-model", false, OpenAIEndpointCapabilityChatCompletions))
	require.False(t, isOpenAICompatibleAccountEligibleForRequest(context.Background(), account, PlatformDevin, "unmapped-model", false, OpenAIEndpointCapabilityChatCompletions))
	for _, capability := range []OpenAIEndpointCapability{OpenAIEndpointCapabilityResponses, OpenAIEndpointCapabilityAlphaSearch, OpenAIEndpointCapabilityEmbeddings, OpenAIEndpointCapabilityLive, OpenAIEndpointCapabilityGrokMediaGeneration, OpenAIEndpointCapabilitySeedance} {
		require.False(t, account.SupportsOpenAIEndpointCapability(capability), string(capability))
	}
	account.Credentials["openai_capabilities"] = map[string]any{"chat_completions": false}
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilityChatCompletions), "honor administrator capability restrictions")
	group := &Group{Platform: PlatformDevin}
	require.Empty(t, group.ResolveMessagesDispatchModel("claude-sonnet-4-5"), "do not inject OpenAI defaults into Devin mapping")
	for _, model := range DefaultDevinModelIDs() {
		platform, ok := DetectModelPlatform(model)
		require.True(t, ok)
		require.Equal(t, PlatformDevin, platform)
	}
}
