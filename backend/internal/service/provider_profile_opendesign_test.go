package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// OpenDesign Go registers one profile that serves Chat Completions and native
// Responses from the same /v1 base URL plus an Anthropic-compatible endpoint at
// the bare domain.
func TestProviderProfileOpenDesign(t *testing.T) {
	profile := LookupProviderProfile(PlatformOpenDesign)
	require.NotNil(t, profile, "open_design must register a provider profile")
	require.Equal(t, AccountModePayG, profile.DefaultMode)
	require.Equal(t, ProviderRoutingByInbound, profile.Routing)

	endpoints, ok := profile.Modes[AccountModePayG]
	require.True(t, ok, "payg mode must be registered")
	require.Equal(t, DefaultOpenDesignBaseURL, endpoints.BaseURLs[APIProtocolChatCompletions])
	require.Equal(t, DefaultOpenDesignBaseURL, endpoints.BaseURLs[APIProtocolResponses])
	require.Equal(t, DefaultOpenDesignAnthropicBaseURL, endpoints.BaseURLs[APIProtocolAnthropic])
}

// Adaptive accounts must resolve endpoints for every inbound protocol.
func TestProviderProfileOpenDesignAdaptiveResolution(t *testing.T) {
	profile := LookupProviderProfile(PlatformOpenDesign)
	require.NotNil(t, profile)

	for protocol, want := range map[string]string{
		APIProtocolChatCompletions: DefaultOpenDesignBaseURL,
		APIProtocolResponses:       DefaultOpenDesignBaseURL,
		APIProtocolAnthropic:       DefaultOpenDesignAnthropicBaseURL,
	} {
		require.True(t, profile.SupportsProtocol(AccountModePayG, protocol), "protocol %s must be supported", protocol)
		require.Equal(t, want, profile.DefaultBaseURL(AccountModePayG, protocol), "protocol %s", protocol)
	}
}
