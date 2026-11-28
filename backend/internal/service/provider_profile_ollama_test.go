package service

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"
)

// Ollama Cloud serves one API key through two protocols: an OpenAI compatible
// endpoint (/v1) and an Anthropic compatible endpoint ({base}/v1/messages).
// It has no native Responses endpoint, so /v1/responses must fall back to Chat
// Completions.
func TestOllamaProviderProfileEndpoints(t *testing.T) {
	profile := LookupProviderProfile(PlatformOllama)
	require.NotNil(t, profile, "ollama must be registered as a multi-protocol provider")

	require.Equal(t, AccountModePayG, profile.DefaultMode)
	require.Equal(t, ProviderRoutingByInbound, profile.Routing)

	require.Equal(t, DefaultOllamaBaseURL, profile.DefaultBaseURL(AccountModePayG, APIProtocolChatCompletions))
	require.Equal(t, DefaultOllamaAnthropicBaseURL, profile.DefaultBaseURL(AccountModePayG, APIProtocolAnthropic))
	require.Empty(t, profile.DefaultBaseURL(AccountModePayG, APIProtocolResponses), "ollama has no native Responses endpoint")

	require.True(t, profile.SupportsProtocol(AccountModePayG, APIProtocolChatCompletions))
	require.True(t, profile.SupportsProtocol(AccountModePayG, APIProtocolAnthropic))
	require.False(t, profile.SupportsProtocol(AccountModePayG, APIProtocolResponses))
}

func TestOllamaPlatformUsesOpenAIGateway(t *testing.T) {
	spec, ok := domain.LookupPlatform(PlatformOllama)
	require.True(t, ok)
	require.Equal(t, PlatformOllama, spec.ID)
	require.Equal(t, "Ollama", spec.DisplayName)
	require.Equal(t, domain.PlatformGatewayOpenAI, spec.Gateway)
}
