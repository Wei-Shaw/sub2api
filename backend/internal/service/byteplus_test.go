//go:build unit

package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestBytePlusProfileAndCapabilities(t *testing.T) {
	profile := LookupProviderProfile(PlatformBytePlus)
	require.NotNil(t, profile)
	require.True(t, IsCNProvider(PlatformBytePlus))
	require.True(t, (&Account{Platform: PlatformBytePlus}).IsOpenAICompatible())
	require.Equal(t, DefaultBytePlusBaseURL, profile.DefaultBaseURL(AccountModePayG, APIProtocolChatCompletions))
	require.False(t, profile.SupportsProtocol(AccountModePayG, APIProtocolAnthropic))
	require.Len(t, profile.Modes, 1)
	account := seedanceTestAccount()
	account.Platform = PlatformBytePlus
	require.True(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance))
	account.Type = AccountTypeOAuth
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance))
	account.Type = AccountTypeAPIKey
	delete(account.Credentials, "openai_capabilities")
	require.False(t, account.SupportsOpenAIEndpointCapability(OpenAIEndpointCapabilitySeedance))
}

func TestBytePlusCompositeModelDetection(t *testing.T) {
	for _, model := range []string{"byteplus/custom-endpoint", "dola-seed-2-1-turbo-260628", "dreamina-seedance-2-5-260628"} {
		platform, ok := DetectModelPlatform(model)
		require.True(t, ok)
		require.Equal(t, PlatformBytePlus, platform)
	}
}

func TestBytePlusTextForwardingUsesArkChatEndpoint(t *testing.T) {
	account := &Account{ID: 91, Platform: PlatformBytePlus, Type: AccountTypeAPIKey, Credentials: map[string]any{
		"base_url": DefaultBytePlusBaseURL, "api_key": "test-only-ark-key", "api_protocol": APIProtocolChatCompletions,
	}}
	body := []byte(`{"model":"dola-seed-2-1-turbo-260628","messages":[{"role":"user","content":"hello"}],"stream":false}`)
	upstream := &httpUpstreamRecorder{err: errors.New("stop after capture")}
	svc := &OpenAIGatewayService{cfg: rawChatCompletionsTestConfig(), httpUpstream: upstream}
	_, err := svc.forwardAsRawChatCompletions(context.Background(), adaptiveProtocolTestContext("/v1/chat/completions", body), account, body, "")
	require.Error(t, err)
	require.NotNil(t, upstream.lastReq)
	require.Equal(t, DefaultBytePlusBaseURL+"/chat/completions", upstream.lastReq.URL.String())
	require.Equal(t, "Bearer test-only-ark-key", upstream.lastReq.Header.Get("Authorization"))
}
